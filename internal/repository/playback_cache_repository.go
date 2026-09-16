package repository

import (
	"context"
	"strconv"
	"time"

	"ripit-be/internal/domain"

	"github.com/redis/go-redis/v9"
)

// playbackCacheTTL bounds how long a room's live playback state lives in
// Redis without being refreshed. A live room gets refreshed every heartbeat
// (well under this), so this really only cleans up rooms nobody is using
// anymore.
const playbackCacheTTL = 6 * time.Hour

type playbackCacheRepository struct {
	rdb *redis.Client
}

func NewPlaybackCacheRepository(rdb *redis.Client) domain.PlaybackCache {
	return &playbackCacheRepository{rdb: rdb}
}

func playbackCacheKey(roomID string) string {
	return "room:" + roomID + ":playback"
}

func (c *playbackCacheRepository) SetPlaybackState(roomID string, state domain.PlaybackState, positionMS int, currentTrackID *string) error {
	ctx := context.Background()
	key := playbackCacheKey(roomID)

	values := map[string]interface{}{
		"playback_state":      string(state),
		"playback_position_ms": positionMS,
		"updated_at":           time.Now().UnixMilli(),
	}
	if currentTrackID != nil {
		values["current_track_id"] = *currentTrackID
	}

	pipe := c.rdb.TxPipeline()
	pipe.HSet(ctx, key, values)
	pipe.Expire(ctx, key, playbackCacheTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *playbackCacheRepository) GetPlaybackState(roomID string) (*domain.PlaybackSnapshot, error) {
	ctx := context.Background()
	key := playbackCacheKey(roomID)

	result, err := c.rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, nil
	}

	positionMS, _ := strconv.Atoi(result["playback_position_ms"])
	updatedAtMs, _ := strconv.ParseInt(result["updated_at"], 10, 64)

	snapshot := &domain.PlaybackSnapshot{
		PlaybackState:      domain.PlaybackState(result["playback_state"]),
		PlaybackPositionMS: positionMS,
		UpdatedAt:          time.UnixMilli(updatedAtMs),
	}
	if trackID, ok := result["current_track_id"]; ok && trackID != "" {
		snapshot.CurrentTrackID = &trackID
	}
	return snapshot, nil
}

func (c *playbackCacheRepository) DeletePlaybackState(roomID string) error {
	return c.rdb.Del(context.Background(), playbackCacheKey(roomID)).Err()
}