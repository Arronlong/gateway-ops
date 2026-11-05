package storage

import "gorm.io/gorm"

const PelicanThumbnailMaxBytes = 96 * 1024
const PelicanThumbnailWidth = 448
const PelicanThumbnailHeight = 336

type PelicanArtifactRef struct {
	ID        uint
	ChannelID uint
	StationID uint
	AccountID int64
	GroupID   int64
}

func (r *RelayStations) pelicanArtifactQuery(ref PelicanArtifactRef) *gorm.DB {
	if ref.GroupID > 0 {
		return r.db.Table("relay_group_pelican_results").Where("id = ? AND relay_station_id = ? AND group_external_id = ?", ref.ID, ref.StationID, ref.GroupID)
	}
	query := r.db.Table("relay_pelican_results").Where("id = ?", ref.ID)
	if ref.ChannelID > 0 {
		return query.Where("channel_id = ?", ref.ChannelID)
	}
	return query.Where("relay_station_id = ? AND account_external_id = ?", ref.StationID, ref.AccountID)
}

func (r *RelayStations) PelicanThumbnail(ref PelicanArtifactRef) ([]byte, error) {
	var result struct {
		ID        uint
		Thumbnail []byte
	}
	err := r.pelicanArtifactQuery(ref).Select("id,thumbnail").Where("status = ?", "success").Take(&result).Error
	if err != nil {
		return nil, err
	}
	if len(result.Thumbnail) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return result.Thumbnail, nil
}

func (r *RelayStations) PelicanThumbnailSource(ref PelicanArtifactRef) (string, error) {
	var result struct {
		ID   uint
		HTML string
	}
	err := r.pelicanArtifactQuery(ref).Select("id,html").Where("status = ?", "success").Take(&result).Error
	return result.HTML, err
}

func (r *RelayStations) SavePelicanThumbnail(ref PelicanArtifactRef, html string, data []byte) error {
	result := r.pelicanArtifactQuery(ref).Where("status = ? AND html = ?", "success", html).Update("thumbnail", data)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
