package service

import (
	"context"

	"github.com/google/uuid"
	"knowledge-base/internal/demo/entity"
)

type Repository interface {
	LatestActiveUser(context.Context) (uuid.UUID, error)
	Seed(context.Context, uuid.UUID, string, ...[]entity.DocumentSeed) (entity.Report, error)
}

type Service struct {
	repository Repository
	fetcher    *ContentFetcher
}

func New(repository Repository) *Service {
	return &Service{repository: repository}
}

// WithContentFetcher 开启受控 GitHub 教程抓取；默认不访问网络，便于离线测试。
func (service *Service) WithContentFetcher(fetcher *ContentFetcher) *Service {
	service.fetcher = fetcher
	return service
}

// Seed 写入本地演示数据；未指定用户时绑定最近登录的真实账号。
func (service *Service) Seed(ctx context.Context, userID uuid.UUID, dataDir string) (entity.Report, error) {
	if userID == uuid.Nil {
		var err error
		userID, err = service.repository.LatestActiveUser(ctx)
		if err != nil {
			return entity.Report{}, err
		}
	}
	var remoteDocuments []entity.DocumentSeed
	if service.fetcher != nil {
		for _, source := range entity.GoTutorialSources() {
			document, err := service.fetcher.Fetch(ctx, source)
			if err != nil {
				return entity.Report{}, err
			}
			remoteDocuments = append(remoteDocuments, document)
		}
	}
	return service.repository.Seed(ctx, userID, dataDir, remoteDocuments)
}
