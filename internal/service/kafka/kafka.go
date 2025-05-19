package kafka

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/segmentio/kafka-go"

	"agregator/rss/internal/model/rss"
)

type Kafka struct {
	writer *kafka.Writer
}

// New создает новый экземпляр Kafka для записи в указанный топик
func New() *Kafka {
	return &Kafka{}
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
				log.Printf("Error encoding item to JSON: %v\n", err)
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
				log.Printf("Error writing message to Kafka: %v\n", err)
			} else {
				log.Printf("Message written to Kafka: %s\n", item.MD5)
			}
		}
	}()
}
