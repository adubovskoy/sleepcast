package youtube

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/kkdai/youtube/v2"
)

// DownloadAudio fetches the lowest-bitrate m4a audio stream for videoID and writes it to destPath.
// Returns (title, error). The caller owns destPath (creates dir, handles cleanup on error).
func DownloadAudio(ctx context.Context, videoID, destPath string) (title string, err error) {
	client := youtube.Client{}

	vid, err := client.GetVideoContext(ctx, videoID)
	if err != nil {
		return "", fmt.Errorf("get video: %w", err)
	}

	audio := make([]youtube.Format, 0, len(vid.Formats))
	for _, f := range vid.Formats {
		if strings.HasPrefix(f.MimeType, "audio/mp4") {
			audio = append(audio, f)
		}
	}
	if len(audio) == 0 {
		return vid.Title, fmt.Errorf("no m4a audio formats available")
	}
	// Lowest bitrate first — itag 139 (~48 kbps) is ideal for sleep listening.
	sort.Slice(audio, func(i, j int) bool { return audio[i].Bitrate < audio[j].Bitrate })
	chosen := audio[0]

	stream, _, err := client.GetStreamContext(ctx, vid, &chosen)
	if err != nil {
		return vid.Title, fmt.Errorf("get stream: %w", err)
	}
	defer stream.Close()

	tmp := destPath + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return vid.Title, err
	}
	if _, err := io.Copy(f, stream); err != nil {
		f.Close()
		os.Remove(tmp)
		return vid.Title, err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return vid.Title, err
	}
	if err := os.Rename(tmp, destPath); err != nil {
		os.Remove(tmp)
		return vid.Title, err
	}
	return vid.Title, nil
}
