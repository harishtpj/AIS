package ontology

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	entitySetKey = "ais:ontology:entities"
	entityKeyFmt = "ais:ontology:entity:%s:%s"
	activeKeyFmt = "ais:ontology:active:%s"
)

type Store struct {
	client *redis.Client
}

func NewStore(ctx context.Context, address, password string, db int) (*Store, error) {
	if strings.TrimSpace(address) == "" {
		address = "127.0.0.1:6379"
	}
	client := redis.NewClient(&redis.Options{Addr: address, Password: password, DB: db})
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connect to ontology Redis: %w", err)
	}
	return &Store{client: client}, nil
}

func (s *Store) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

func (s *Store) SaveDocument(ctx context.Context, document *Document) error {
	if err := document.Validate(); err != nil {
		return err
	}
	for _, entity := range document.Entities {
		if err := s.SaveEntity(ctx, entity); err != nil {
			return fmt.Errorf("save entity %q: %w", entity.Name, err)
		}
	}
	return nil
}

func (s *Store) SaveEntity(ctx context.Context, entity Entity) error {
	if err := (&Document{Version: entity.Version, Entities: []Entity{entity}}).Validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(entity)
	if err != nil {
		return fmt.Errorf("encode entity: %w", err)
	}
	key := fmt.Sprintf(entityKeyFmt, entity.Name, entity.Version)
	activeKey := fmt.Sprintf(activeKeyFmt, entity.Name)
	_, err = s.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Set(ctx, key, raw, 0)
		pipe.SAdd(ctx, entitySetKey, entity.Name)
		pipe.Set(ctx, activeKey, entity.Version, 0)
		return nil
	})
	if err != nil {
		return fmt.Errorf("persist entity: %w", err)
	}
	return nil
}

func (s *Store) GetEntity(ctx context.Context, name, version string) (*Entity, error) {
	name = strings.TrimSpace(name)
	version = strings.TrimSpace(version)
	if name == "" {
		return nil, fmt.Errorf("entity name cannot be empty")
	}
	if version == "" {
		var err error
		version, err = s.client.Get(ctx, fmt.Sprintf(activeKeyFmt, name)).Result()
		if err != nil {
			return nil, fmt.Errorf("get active version for entity %q: %w", name, err)
		}
	}
	raw, err := s.client.Get(ctx, fmt.Sprintf(entityKeyFmt, name, version)).Bytes()
	if err != nil {
		return nil, fmt.Errorf("get entity %q version %q: %w", name, version, err)
	}
	var entity Entity
	if err := json.Unmarshal(raw, &entity); err != nil {
		return nil, fmt.Errorf("decode entity %q version %q: %w", name, version, err)
	}
	return &entity, nil
}
