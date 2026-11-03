package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"gorm.io/gorm"
)

const dbChunkSize = 2 * 1024 * 1024 // 2MB chunk size for database blob storage

func init() {
	RegisterDriver("database", func(ctx context.Context, policy model.StoragePolicy, db *gorm.DB) (StorageDriver, error) {
		if db == nil {
			return nil, errors.New("database connection not available")
		}
		return NewDatabaseStorageDriver(db), nil
	})
}

type DatabaseStorageDriver struct {
	db *gorm.DB
}

func NewDatabaseStorageDriver(db *gorm.DB) *DatabaseStorageDriver {
	return &DatabaseStorageDriver{db: db}
}

func (d *DatabaseStorageDriver) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	if d.db == nil {
		return errors.New("database not available")
	}

	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Clean up existing chunks if overwriting
		if err := tx.Where("storage_key = ?", key).Delete(&model.FileBlob{}).Error; err != nil {
			return err
		}

		buf := make([]byte, dbChunkSize)
		chunkIndex := 0

		for {
			n, err := io.ReadFull(r, buf)
			if n > 0 {
				chunkData := make([]byte, n)
				copy(chunkData, buf[:n])

				blob := model.FileBlob{
					StorageKey: key,
					ChunkIndex: chunkIndex,
					Data:       chunkData,
					Size:       int64(n),
					CreatedAt:  time.Now(),
				}
				if createErr := tx.Create(&blob).Error; createErr != nil {
					return createErr
				}
				chunkIndex++
			}
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (d *DatabaseStorageDriver) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if d.db == nil {
		return nil, errors.New("database not available")
	}

	var count int64
	if err := d.db.WithContext(ctx).Model(&model.FileBlob{}).Where("storage_key = ?", key).Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, errors.New("file blob not found in database")
	}

	return &dbBlobReader{
		ctx:        ctx,
		db:         d.db,
		key:        key,
		chunkIndex: 0,
		total:      int(count),
	}, nil
}

type dbBlobReader struct {
	ctx        context.Context
	db         *gorm.DB
	key        string
	chunkIndex int
	total      int
	currentBuf *bytes.Reader
}

func (r *dbBlobReader) Read(p []byte) (n int, err error) {
	for {
		if r.currentBuf != nil && r.currentBuf.Len() > 0 {
			return r.currentBuf.Read(p)
		}

		if r.chunkIndex >= r.total {
			return 0, io.EOF
		}

		var blob model.FileBlob
		err := r.db.WithContext(r.ctx).
			Select("data").
			Where("storage_key = ? AND chunk_index = ?", r.key, r.chunkIndex).
			First(&blob).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return 0, io.EOF
			}
			return 0, err
		}

		r.currentBuf = bytes.NewReader(blob.Data)
		r.chunkIndex++
	}
}

func (r *dbBlobReader) Close() error {
	r.currentBuf = nil
	return nil
}

func (d *DatabaseStorageDriver) Delete(ctx context.Context, key string) error {
	if d.db == nil {
		return errors.New("database not available")
	}
	return d.db.WithContext(ctx).Where("storage_key = ?", key).Delete(&model.FileBlob{}).Error
}

func (d *DatabaseStorageDriver) Stat(ctx context.Context, key string) (FileInfo, error) {
	if d.db == nil {
		return FileInfo{}, errors.New("database not available")
	}

	type statResult struct {
		TotalSize  int64 `gorm:"column:total_size"`
		ChunkCount int64 `gorm:"column:chunk_count"`
	}

	var res statResult
	err := d.db.WithContext(ctx).Model(&model.FileBlob{}).
		Select("COALESCE(SUM(size), 0) AS total_size, COUNT(*) AS chunk_count").
		Where("storage_key = ?", key).
		Scan(&res).Error
	if err != nil {
		return FileInfo{}, err
	}
	if res.ChunkCount == 0 {
		return FileInfo{}, errors.New("file not found in database")
	}

	var latestBlob model.FileBlob
	_ = d.db.WithContext(ctx).Select("created_at").Where("storage_key = ?", key).Order("chunk_index DESC").First(&latestBlob).Error

	return FileInfo{
		Size:         res.TotalSize,
		LastModified: latestBlob.CreatedAt,
	}, nil
}

func (d *DatabaseStorageDriver) Test(ctx context.Context) error {
	if d.db == nil {
		return errors.New("database not available")
	}
	var probe int
	return d.db.WithContext(ctx).Raw("SELECT 1").Scan(&probe).Error
}

func (d *DatabaseStorageDriver) GetLocalPath(key string) (string, bool) {
	return "", false
}
