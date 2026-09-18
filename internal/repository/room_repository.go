package repository

import (
	"errors"
	"time"

	"ripit-be/internal/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type roomRepository struct {
	db *gorm.DB
}

func NewRoomRepository(db *gorm.DB) domain.RoomRepository {
	return &roomRepository{db: db}
}

func (r *roomRepository) Create(room *domain.Room) error {
	return r.db.Create(room).Error
}

func (r *roomRepository) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&domain.Room{}).Error
}

func (r *roomRepository) GetByID(id string) (*domain.Room, error) {
	var room domain.Room
	err := r.db.Preload("Host").
		Preload("CurrentTrack").
		Preload("Members.User").
		Preload("Tracks", func(db *gorm.DB) *gorm.DB {
			return db.Order("sort_order ASC, created_at ASC")
		}).
		First(&room, "id = ?", id).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &room, nil
}

func (r *roomRepository) GetByCode(code string) (*domain.Room, error) {
	var room domain.Room
	err := r.db.Preload("Host").
		Preload("CurrentTrack").
		Preload("Members.User").
		Preload("Tracks", func(db *gorm.DB) *gorm.DB {
			return db.Order("sort_order ASC, created_at ASC")
		}).
		First(&room, "room_code = ?", code).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &room, nil
}

func (r *roomRepository) Update(room *domain.Room) error {
	return r.db.Save(room).Error
}

func (r *roomRepository) UpdatePlaybackState(roomID string, state domain.PlaybackState, positionMS *int, currentTrackID *string) error {
	updates := map[string]interface{}{
		"playback_state":      state,
		"last_sync_timestamp": time.Now(),
	}
	if positionMS != nil {
		updates["playback_position_ms"] = *positionMS
	}
	if currentTrackID != nil {
		updates["current_track_id"] = *currentTrackID
	}
	return r.db.Model(&domain.Room{}).Where("id = ?", roomID).Updates(updates).Error
}

func (r *roomRepository) UpdatePlaybackSettings(roomID string, repeatMode string, isShuffled bool) error {
	return r.db.Model(&domain.Room{}).
		Where("id = ?", roomID).
		Updates(map[string]interface{}{
			"repeat_mode": repeatMode,
			"is_shuffled": isShuffled,
		}).Error
}

// AddMember upserts room membership: inserts a new row, or — if the user is
// already a member (e.g. reconnecting after a page refresh) — updates their
// role/joined_at in place instead of erroring or creating a duplicate.
func (r *roomRepository) AddMember(member *domain.RoomMember) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "room_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"role", "joined_at"}),
	}).Create(member).Error
}

func (r *roomRepository) RemoveMember(roomID, userID string) error {
	return r.db.Where("room_id = ? AND user_id = ?", roomID, userID).Delete(&domain.RoomMember{}).Error
}

func (r *roomRepository) GetMembers(roomID string) ([]domain.RoomMember, error) {
	var members []domain.RoomMember
	err := r.db.Preload("User").Where("room_id = ?", roomID).Find(&members).Error
	return members, err
}

func (r *roomRepository) GetMember(roomID, userID string) (*domain.RoomMember, error) {
	var member domain.RoomMember
	err := r.db.Preload("User").Where("room_id = ? AND user_id = ?", roomID, userID).First(&member).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &member, nil
}

func (r *roomRepository) IsCodeExists(code string) (bool, error) {
	var count int64
	err := r.db.Model(&domain.Room{}).Where("room_code = ?", code).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *roomRepository) MarkSessionStarted(roomID string) (*time.Time, error) {
	now := time.Now()
	result := r.db.Model(&domain.Room{}).
		Where("id = ? AND session_started_at IS NULL", roomID).
		Update("session_started_at", now)

	if result.Error != nil {
		return nil, result.Error
	}

	if result.RowsAffected == 0 {
		var room domain.Room
		if err := r.db.Select("session_started_at").First(&room, "id = ?", roomID).Error; err != nil {
			return nil, err
		}
		return room.SessionStartedAt, nil
	}

	return &now, nil
}

func (r *roomRepository) TransferHost(roomID, oldHostID, newHostID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&domain.Room{}).
			Where("id = ?", roomID).
			Update("host_id", newHostID).Error; err != nil {
			return err
		}

		if err := tx.Model(&domain.RoomMember{}).
			Where("room_id = ? AND user_id = ?", roomID, newHostID).
			Update("role", domain.RoomRoleHost).Error; err != nil {
			return err
		}

		if err := tx.Where("room_id = ? AND user_id = ?", roomID, oldHostID).
			Delete(&domain.RoomMember{}).Error; err != nil {
			return err
		}

		return nil
	})
}