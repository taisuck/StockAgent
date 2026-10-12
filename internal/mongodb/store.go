// Package mongodb는 MongoDB에 연결해 수집 대상 종목 목록을 읽는다.
package mongodb

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/taisuck/StockAgent/internal/config"
)

// CorporationCollection은 수집 대상 회사가 담긴 컬렉션 이름이다.
const CorporationCollection = "corporation"

// timeout은 연결 확인과 조회 한 번에 허용하는 시간이다.
const timeout = 10 * time.Second

// corporation은 corporation 컬렉션 문서 중 수집에 쓰는 필드다.
type corporation struct {
	Code string `bson:"code"`
	Name string `bson:"name"`
}

// Store는 MongoDB 연결 하나를 감싼다.
type Store struct {
	client *mongo.Client
	corps  *mongo.Collection
	log    *slog.Logger
}

// Connect는 cfg로 MongoDB에 연결하고 ping으로 접속·인증을 확인한다.
// username이 있으면 URI의 인증 정보 대신 그것을 쓴다(authSource 등 URI의 다른 인증 옵션은 유지).
func Connect(ctx context.Context, cfg config.Mongo, log *slog.Logger) (*Store, error) {
	opts := options.Client().ApplyURI(cfg.URI).SetTimeout(timeout)
	if cfg.Username != "" {
		cred := options.Credential{}
		if opts.Auth != nil {
			cred = *opts.Auth
		}
		cred.Username = cfg.Username
		cred.Password = cfg.Password
		cred.PasswordSet = true
		opts.SetAuth(cred)
	}

	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, fmt.Errorf("MongoDB 연결 설정 실패: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		client.Disconnect(context.Background())
		return nil, fmt.Errorf("MongoDB 접속 실패: %w", err)
	}

	return &Store{
		client: client,
		corps:  client.Database(cfg.Database).Collection(CorporationCollection),
		log:    log,
	}, nil
}

// Symbols는 corporation 컬렉션의 code, name을 수집 대상 종목으로 돌려준다.
// code나 name이 비어 있는 문서는 metric label을 만들 수 없으므로 건너뛴다.
func (s *Store) Symbols(ctx context.Context) ([]config.Symbol, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	find := options.Find().
		SetProjection(bson.D{{Key: "code", Value: 1}, {Key: "name", Value: 1}}).
		SetSort(bson.D{{Key: "code", Value: 1}})
	cur, err := s.corps.Find(ctx, bson.D{}, find)
	if err != nil {
		return nil, fmt.Errorf("%s 조회 실패: %w", CorporationCollection, err)
	}

	var docs []corporation
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("%s 문서 읽기 실패: %w", CorporationCollection, err)
	}

	symbols := make([]config.Symbol, 0, len(docs))
	for _, d := range docs {
		if d.Code == "" || d.Name == "" {
			s.log.Warn("code 또는 name이 비어 있는 문서를 건너뜀", "code", d.Code, "name", d.Name)
			continue
		}
		symbols = append(symbols, config.Symbol{Code: d.Code, Name: d.Name})
	}
	return symbols, nil
}

// Close는 연결을 닫는다.
func (s *Store) Close(ctx context.Context) error {
	return s.client.Disconnect(ctx)
}
