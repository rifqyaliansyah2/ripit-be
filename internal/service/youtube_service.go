package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ripit-be/internal/domain"
)

type youtubeService struct {
	apiKey     string
	httpClient *http.Client
}

type oEmbedResponse struct {
	Title        string `json:"title"`
	AuthorName   string `json:"author_name"`
	AuthorURL    string `json:"author_url"`
	ThumbnailURL string `json:"thumbnail_url"`
}

type youtubeAPIResponse struct {
	Items []struct {
		Snippet struct {
			Title        string `json:"title"`
			ChannelTitle string `json:"channelTitle"`
			Thumbnails   struct {
				High struct {
					URL string `json:"url"`
				} `json:"high"`
				Default struct {
					URL string `json:"url"`
				} `json:"default"`
			} `json:"thumbnails"`
		} `json:"snippet"`
		ContentDetails struct {
			Duration string `json:"duration"` // ISO 8601 e.g. PT3M45S
		} `json:"contentDetails"`
	} `json:"items"`
}

func NewYouTubeService(apiKey string) domain.YouTubeService {
	return &youtubeService{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *youtubeService) extractVideoID(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", errors.New("invalid URL format")
	}

	// youtu.be/VIDEO_ID
	if strings.Contains(parsed.Host, "youtu.be") {
		id := strings.TrimPrefix(parsed.Path, "/")
		if id != "" {
			return id, nil
		}
	}

	// youtube.com/watch?v=VIDEO_ID or youtube.com/embed/VIDEO_ID or youtube.com/shorts/VIDEO_ID
	if strings.Contains(parsed.Host, "youtube.com") {
		if v := parsed.Query().Get("v"); v != "" {
			return v, nil
		}
		if strings.HasPrefix(parsed.Path, "/embed/") {
			return strings.TrimPrefix(parsed.Path, "/embed/"), nil
		}
		if strings.HasPrefix(parsed.Path, "/shorts/") {
			return strings.TrimPrefix(parsed.Path, "/shorts/"), nil
		}
	}

	return "", errors.New("could not extract YouTube video ID from URL")
}

func parseISODuration(isoDuration string) (string, int) {
	// e.g., PT3M45S, PT1H2M10S, PT45S
	re := regexp.MustCompile(`PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?`)
	matches := re.FindStringSubmatch(isoDuration)
	if len(matches) == 0 {
		return "0:00", 0
	}

	hours, _ := strconv.Atoi(matches[1])
	mins, _ := strconv.Atoi(matches[2])
	secs, _ := strconv.Atoi(matches[3])

	totalSecs := hours*3600 + mins*60 + secs
	var formatted string
	if hours > 0 {
		formatted = fmt.Sprintf("%d:%02d:%02d", hours, mins, secs)
	} else {
		formatted = fmt.Sprintf("%d:%02d", mins, secs)
	}

	return formatted, totalSecs
}

func (s *youtubeService) CheckAndFetchMetadata(rawURL string) (*domain.YouTubeMetadataResponse, error) {
	videoID, err := s.extractVideoID(rawURL)
	if err != nil {
		return nil, err
	}

	standardURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)
	defaultCover := fmt.Sprintf("https://img.youtube.com/vi/%s/hqdefault.jpg", videoID)

	// 1. If API key is available, fetch via YouTube Data API v3
	if s.apiKey != "" {
		apiURL := fmt.Sprintf("https://www.googleapis.com/youtube/v3/videos?id=%s&key=%s&part=snippet,contentDetails", videoID, s.apiKey)
		resp, err := s.httpClient.Get(apiURL)
		if err == nil && resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			var ytData youtubeAPIResponse
			if err := json.NewDecoder(resp.Body).Decode(&ytData); err == nil && len(ytData.Items) > 0 {
				item := ytData.Items[0]
				formattedDuration, durationSec := parseISODuration(item.ContentDetails.Duration)
				cover := item.Snippet.Thumbnails.High.URL
				if cover == "" {
					cover = defaultCover
				}

				return &domain.YouTubeMetadataResponse{
					VideoID:     videoID,
					Title:       item.Snippet.Title,
					Artist:      item.Snippet.ChannelTitle,
					Duration:    formattedDuration,
					DurationSec: durationSec,
					CoverURL:    cover,
					YouTubeURL:  standardURL,
				}, nil
			}
		}
	}

	// 2. Fallback to YouTube oEmbed endpoint (no API key required)
	oembedURL := fmt.Sprintf("https://www.youtube.com/oembed?url=%s&format=json", url.QueryEscape(standardURL))
	resp, err := s.httpClient.Get(oembedURL)
	if err != nil {
		return nil, fmt.Errorf("failed to contact YouTube oEmbed service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("youtube video not found or is private (status %d)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var oembed oEmbedResponse
	if err := json.Unmarshal(body, &oembed); err != nil {
		return nil, fmt.Errorf("failed to parse YouTube oembed response: %w", err)
	}

	cover := oembed.ThumbnailURL
	if cover == "" {
		cover = defaultCover
	}

	return &domain.YouTubeMetadataResponse{
		VideoID:     videoID,
		Title:       oembed.Title,
		Artist:      oembed.AuthorName,
		Duration:    "0:00", // oEmbed does not return duration
		DurationSec: 0,
		CoverURL:    cover,
		YouTubeURL:  standardURL,
	}, nil
}
