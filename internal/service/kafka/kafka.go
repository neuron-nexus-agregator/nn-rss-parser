package kafka

import (
	"context"
	"encoding/json"
	"time"

	"agregator/rss/internal/interfaces"
	"agregator/rss/internal/model/rss"

	"github.com/segmentio/kafka-go"
)

type Kafka struct {
	writer *kafka.Writer
	logger interfaces.Logger
}

// New создает новый экземпляр Kafka для записи в указанный топик
func New(logger interfaces.Logger) *Kafka {
	return &Kafka{
		logger: logger,
	}
}

// StartWriting принимает канал rss.Item и записывает данные в Kafka
func (k *Kafka) StartWriting(brokers []string, topic string, input <-chan rss.Item) {
	k.writer = &kafka.Writer{
		Addr:     kafka.TCP(brokers...),
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},
	}
	go func() {
		for item := range input {
			// Сериализуем rss.Item в JSON
			data, err := json.Marshal(item)
			if err != nil {
				k.logger.Error("Error marshaling rss.Item to JSON", "error", err)
				continue
			}

			// Создаем сообщение для Kafka
			message := kafka.Message{
				Key:   []byte(item.MD5), // Используем ID как ключ
				Value: data,
			}

			// Устанавливаем контекст с таймаутом для записи
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

			// Пишем сообщение в Kafka
			err = k.writer.WriteMessages(ctx, message)
			cancel()
			if err != nil {
				k.logger.Error("Error writing message to Kafka", "error", err)
			}
		}
	}()
}
