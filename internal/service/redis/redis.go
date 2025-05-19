package redis

import (
	"github.com/go-redis/redis"

	"agregator/rss/internal/config"
)

type RedisCache struct {
	client *redis.Client
}

func New(addr string, password string) *RedisCache {
	return &RedisCache{
		client: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       0,
		}),
	}
}

// Set записывает ключ-значение в кэш с временем жизни 4 дня
func (r *RedisCache) Set(key string, value string) error {
	// Устанавливаем ключ-значение в кэш
	err := r.client.Set(key, value, config.CacheTTL).Err()
	if err != nil {
		return err
	}

	return nil
}

// Exists проверяет наличие ключа в кэше и возвращает его значение
func (r *RedisCache) Exists(key string) (bool, string, error) {
	// Получаем значение по ключу
	value, err := r.client.Get(key).Result()
	if err == redis.Nil {
		// Ключ не найден
		return false, "", nil
	} else if err != nil {
		// Произошла ошибка при обращении к Redis
		return false, "", err
	}

	// Ключ найден, возвращаем true и значение
	return true, value, nil
}
