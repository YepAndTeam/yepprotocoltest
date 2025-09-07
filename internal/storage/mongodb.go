package storage

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoDB struct {
	client   *mongo.Client
	database *mongo.Database
	messages *mongo.Collection
}

// Message структура для MongoDB
type MongoMessage struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	FromYUI   string             `bson:"from_yui"`
	ToYUI     string             `bson:"to_yui,omitempty"`
	Content   string             `bson:"content"`
	Level     string             `bson:"level"`
	Encrypted bool               `bson:"encrypted"`
	CreatedAt time.Time          `bson:"created_at"`
	IsRead    bool               `bson:"is_read"`
}

// Подключение к MongoDB
func NewMongoDB(uri string) (*MongoDB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Подключаемся
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Проверяем подключение
	err = client.Ping(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	database := client.Database("yep_hub")
	messages := database.Collection("messages")

	// Создаём индексы для быстрого поиска
	_, err = messages.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{"from_yui", 1}}},
		{Keys: bson.D{{"to_yui", 1}}},
		{Keys: bson.D{{"created_at", -1}}},
	})

	log.Println("✅ Connected to MongoDB")

	return &MongoDB{
		client:   client,
		database: database,
		messages: messages,
	}, nil
}

// Сохранить сообщение
func (m *MongoDB) SaveMessage(msg *MongoMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msg.CreatedAt = time.Now()

	result, err := m.messages.InsertOne(ctx, msg)
	if err != nil {
		return fmt.Errorf("failed to save message: %w", err)
	}

	log.Printf("📝 Message saved with ID: %v", result.InsertedID)
	return nil
}

// Получить историю сообщений
func (m *MongoDB) GetMessageHistory(yui string, limit int64) ([]*MongoMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Фильтр: сообщения от или для пользователя
	filter := bson.M{
		"$or": []bson.M{
			{"from_yui": yui},
			{"to_yui": yui},
		},
	}

	// Опции: сортировка по времени, лимит
	opts := options.Find().
		SetSort(bson.D{{"created_at", -1}}).
		SetLimit(limit)

	cursor, err := m.messages.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var messages []*MongoMessage
	if err = cursor.All(ctx, &messages); err != nil {
		return nil, err
	}

	return messages, nil
}

// Получить непрочитанные сообщения
func (m *MongoDB) GetUnreadMessages(yui string) ([]*MongoMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{
		"to_yui":  yui,
		"is_read": false,
	}

	cursor, err := m.messages.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var messages []*MongoMessage
	if err = cursor.All(ctx, &messages); err != nil {
		return nil, err
	}

	return messages, nil
}

// Пометить как прочитанное
func (m *MongoDB) MarkAsRead(messageID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	objID, err := primitive.ObjectIDFromHex(messageID)
	if err != nil {
		return err
	}

	filter := bson.M{"_id": objID}
	update := bson.M{"$set": bson.M{"is_read": true}}

	_, err = m.messages.UpdateOne(ctx, filter, update)
	return err
}

// Статистика сообщений
func (m *MongoDB) GetMessageStats(yui string) (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Подсчёт отправленных
	sentCount, err := m.messages.CountDocuments(ctx, bson.M{"from_yui": yui})
	if err != nil {
		return nil, err
	}

	// Подсчёт полученных
	receivedCount, err := m.messages.CountDocuments(ctx, bson.M{"to_yui": yui})
	if err != nil {
		return nil, err
	}

	// Подсчёт непрочитанных
	unreadCount, err := m.messages.CountDocuments(ctx, bson.M{
		"to_yui":  yui,
		"is_read": false,
	})

	return map[string]interface{}{
		"sent":     sentCount,
		"received": receivedCount,
		"unread":   unreadCount,
	}, nil
}

// Закрыть подключение
func (m *MongoDB) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return m.client.Disconnect(ctx)
}
