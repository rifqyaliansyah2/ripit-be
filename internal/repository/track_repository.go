package repository

import (
	"errors"

	"ripit-be/internal/domain"

	"gorm.io/gorm"
)

type trackRepository struct {
	db *gorm.DB
}

func NewTrackRepository(db *gorm.DB) domain.TrackRepository {
	return &trackRepository{db: db}
}

func (r *trackRepository) Create(track *domain.Track) error {
	return r.db.Create(track).Error
}

func (r *trackRepository) GetByID(id string) (*domain.Track, error) {
	var track domain.Track
	err := r.db.Preload("User").First(&track, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &track, nil
}

func (r *trackRepository) GetByRoomID(roomID string) ([]domain.Track, error) {
	var tracks []domain.Track
	err := r.db.Preload("User").
		Where("room_id = ?", roomID).
		Order("sort_order ASC, created_at ASC").
		Find(&tracks).Error
	return tracks, err
}

func (r *trackRepository) Update(track *domain.Track) error {
	return r.db.Save(track).Error
}

func (r *trackRepository) Delete(id string) error {
	return r.db.Delete(&domain.Track{}, "id = ?", id).Error
}

func (r *trackRepository) GetMaxSortOrder(roomID string) (int, error) {
	var maxOrder *int
	row := r.db.Model(&domain.Track{}).Where("room_id = ?", roomID).Select("MAX(sort_order)").Row()
	if err := row.Scan(&maxOrder); err != nil {
		return 0, err
	}
	if maxOrder == nil {
		return -1, nil
	}
	return *maxOrder, nil
}

func (r *trackRepository) UpdateSortOrders(roomID string, trackOrderMap map[string]int) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		for trackID, sortOrder := range trackOrderMap {
			if err := tx.Model(&domain.Track{}).
				Where("id = ? AND room_id = ?", trackID, roomID).
				Update("sort_order", sortOrder).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
