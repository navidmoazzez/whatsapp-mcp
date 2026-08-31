package wa

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// mediaDownloadable adapts stored media metadata back into the shape whatsmeow
// needs to fetch and decrypt an attachment.
//
// WhatsApp never sends the file itself. It sends a URL, a media key and two
// hashes, and the bytes are fetched and decrypted separately. That is why
// attachments have to be resolved on demand rather than arriving with the
// message.
type mediaDownloadable struct {
	directPath string
	mediaKey   []byte
	fileSHA    []byte
	fileEncSHA []byte
	fileLength uint64
	mediaType  whatsmeow.MediaType
}

func (d *mediaDownloadable) GetDirectPath() string    { return d.directPath }
func (d *mediaDownloadable) GetMediaKey() []byte      { return d.mediaKey }
func (d *mediaDownloadable) GetFileSHA256() []byte    { return d.fileSHA }
func (d *mediaDownloadable) GetFileEncSHA256() []byte { return d.fileEncSHA }
func (d *mediaDownloadable) GetFileLength() uint64    { return d.fileLength }

// DownloadMedia fetches and decrypts an attachment, writes it into the data
// directory and returns the path.
//
// Unlike the implementation this replaces, the caller does not have to know
// the media type or hunt for a direct path. Everything needed was stored when
// the message arrived.
func (c *Client) DownloadMedia(ctx context.Context, chatJID, messageID string) (string, error) {
	m, err := c.st.GetMedia(ctx, chatJID, messageID)
	if err != nil {
		return "", err
	}
	if m.MediaType == "" {
		return "", fmt.Errorf("message %s has no attachment", messageID)
	}
	if m.MediaPath != "" {
		if _, err := os.Stat(m.MediaPath); err == nil {
			return m.MediaPath, nil // already downloaded
		}
	}

	var kind whatsmeow.MediaType
	switch m.MediaType {
	case "image":
		kind = whatsmeow.MediaImage
	case "video":
		kind = whatsmeow.MediaVideo
	case "audio":
		kind = whatsmeow.MediaAudio
	default:
		kind = whatsmeow.MediaDocument
	}

	data, err := c.wm.Download(ctx, &mediaDownloadable{
		directPath: directPath(m.MediaURL),
		mediaKey:   m.MediaKey,
		fileSHA:    m.FileSHA256,
		fileEncSHA: m.FileEncSHA,
		fileLength: m.FileLength,
		mediaType:  kind,
	})
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}

	dir := filepath.Join(c.dataDir, "media", sanitise(chatJID))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}

	name := m.Filename
	if name == "" {
		name = messageID + extensionFor(m.MediaType)
	}
	path := filepath.Join(dir, sanitise(name))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}

	_ = c.st.SetMediaPath(ctx, chatJID, messageID, path)
	return path, nil
}

// SendFile sends a local file as an image, video, document or audio message.
func (c *Client) SendFile(ctx context.Context, chatJID, path, caption string) (string, error) {
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return "", fmt.Errorf("%q is not a valid chat id: %w", chatJID, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("could not read %s: %w", path, err)
	}

	kind, mediaType := kindFor(path)
	up, err := c.wm.Upload(ctx, data, mediaType)
	if err != nil {
		return "", fmt.Errorf("upload failed: %w", err)
	}

	mime := mimeFor(path)
	msg := &waE2E.Message{}

	switch kind {
	case "image":
		msg.ImageMessage = &waE2E.ImageMessage{
			URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256,
			FileLength: &up.FileLength, Mimetype: &mime,
			Caption: proto.String(caption),
		}
	case "video":
		msg.VideoMessage = &waE2E.VideoMessage{
			URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256,
			FileLength: &up.FileLength, Mimetype: &mime,
			Caption: proto.String(caption),
		}
	case "audio":
		msg.AudioMessage = &waE2E.AudioMessage{
			URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256,
			FileLength: &up.FileLength, Mimetype: &mime,
		}
	default:
		msg.DocumentMessage = &waE2E.DocumentMessage{
			URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256,
			FileLength: &up.FileLength, Mimetype: &mime,
			FileName: proto.String(filepath.Base(path)),
			Caption:  proto.String(caption),
		}
	}

	resp, err := c.wm.SendMessage(ctx, jid, msg)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// SendVoiceBytes sends Ogg Opus audio already in memory as a voice note,
// without it ever touching disk. Generated speech has no reason to be written
// to a file first.
func (c *Client) SendVoiceBytes(ctx context.Context, chatJID string, ogg []byte) (string, error) {
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return "", fmt.Errorf("%q is not a valid chat id: %w", chatJID, err)
	}
	if len(ogg) == 0 {
		return "", fmt.Errorf("no audio to send")
	}

	up, err := c.wm.Upload(ctx, ogg, whatsmeow.MediaAudio)
	if err != nil {
		return "", fmt.Errorf("upload failed: %w", err)
	}

	mime := "audio/ogg; codecs=opus"
	resp, err := c.wm.SendMessage(ctx, jid, &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256,
			FileLength: &up.FileLength, Mimetype: &mime,
			PTT: proto.Bool(true), // PTT is what makes it a voice note
		},
	})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// SendVoiceNote sends an Ogg Opus file as a playable voice message.
//
// WhatsApp only renders the waveform player for Ogg Opus. Anything else
// arrives as a file attachment, so this refuses other formats with an error
// that says how to convert rather than silently sending something that looks
// wrong in the chat.
func (c *Client) SendVoiceNote(ctx context.Context, chatJID, path string) (string, error) {
	if !strings.EqualFold(filepath.Ext(path), ".ogg") {
		return "", fmt.Errorf("a voice note must be Ogg Opus. Convert it first:\n  ffmpeg -i %q -c:a libopus -b:a 32k -ar 24000 -application voip out.ogg", path)
	}

	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return "", fmt.Errorf("%q is not a valid chat id: %w", chatJID, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("could not read %s: %w", path, err)
	}

	up, err := c.wm.Upload(ctx, data, whatsmeow.MediaAudio)
	if err != nil {
		return "", fmt.Errorf("upload failed: %w", err)
	}

	mime := "audio/ogg; codecs=opus"
	resp, err := c.wm.SendMessage(ctx, jid, &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256,
			FileLength: &up.FileLength, Mimetype: &mime,
			PTT: proto.Bool(true), // PTT is what makes it a voice note, not a file
		},
	})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// directPath strips the CDN host off a stored media URL, because whatsmeow
// wants the path portion rather than the full URL.
func directPath(url string) string {
	if i := strings.Index(url, ".net/"); i != -1 {
		if q := strings.Index(url[i+4:], "?"); q != -1 {
			return url[i+4 : i+4+q]
		}
		return url[i+4:]
	}
	return url
}

func kindFor(path string) (string, whatsmeow.MediaType) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return "image", whatsmeow.MediaImage
	case ".mp4", ".mov", ".avi", ".mkv", ".webm":
		return "video", whatsmeow.MediaVideo
	case ".ogg", ".mp3", ".m4a", ".wav", ".aac", ".opus":
		return "audio", whatsmeow.MediaAudio
	}
	return "document", whatsmeow.MediaDocument
}

func mimeFor(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".ogg", ".opus":
		return "audio/ogg; codecs=opus"
	case ".mp3":
		return "audio/mpeg"
	case ".m4a":
		return "audio/mp4"
	case ".wav":
		return "audio/wav"
	case ".pdf":
		return "application/pdf"
	}
	return "application/octet-stream"
}

func extensionFor(mediaType string) string {
	switch mediaType {
	case "image":
		return ".jpg"
	case "video":
		return ".mp4"
	case "audio":
		return ".ogg"
	}
	return ".bin"
}

// sanitise keeps a filename safe to join onto a path.
func sanitise(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "..", "_")
	s = strings.ReplaceAll(s, string(os.PathSeparator), "_")
	if s == "" {
		return "file"
	}
	return s
}

// SendAudioBytes sends audio held in memory as an ordinary audio message
// rather than a voice note.
//
// Used when the bytes are not Ogg Opus. WhatsApp would still accept them
// marked as a voice note, but the player would show a waveform for audio it
// cannot decode, so it arrives silent. An audio file that plays is better than
// a voice note that does not.
func (c *Client) SendAudioBytes(ctx context.Context, chatJID string, data []byte, filename string) (string, error) {
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return "", fmt.Errorf("%q is not a valid chat id: %w", chatJID, err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("no audio to send")
	}

	up, err := c.wm.Upload(ctx, data, whatsmeow.MediaAudio)
	if err != nil {
		return "", fmt.Errorf("upload failed: %w", err)
	}

	mime := mimeFor(filename)
	if !strings.HasPrefix(mime, "audio/") {
		mime = "audio/mpeg"
	}

	resp, err := c.wm.SendMessage(ctx, jid, &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL: &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256,
			FileLength: &up.FileLength, Mimetype: &mime,
			PTT: proto.Bool(false),
		},
	})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}
