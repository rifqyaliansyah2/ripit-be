package domain

type YouTubeCheckRequest struct {
	URL string `json:"url" binding:"required,url"`
}

type YouTubeMetadataResponse struct {
	VideoID     string `json:"video_id"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Duration    string `json:"duration"`
	DurationSec int    `json:"duration_sec"`
	CoverURL    string `json:"cover_url"`
	YouTubeURL  string `json:"youtube_url"`
}

type YouTubeService interface {
	CheckAndFetchMetadata(rawURL string) (*YouTubeMetadataResponse, error)
}
