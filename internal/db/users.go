/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package db

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Users represents a user document in the database.
type Users struct {
	ID        int64  `bson:"_id"`
	FirstName string `bson:"first_name,omitempty"`
	LastName  string `bson:"last_name,omitempty"`
	Username  string `bson:"username,omitempty"`
	UpdatedAt int64  `bson:"updated_at,omitempty"`
}

// AddUser adds a new user to the database if they do not already exist.
func (db *Database) AddUser(userID int64) error {
	return db.SaveUserProfile(userID, "", "", "")
}

// SaveUserProfile upserts a user's Telegram identity after they send /start to the bot.
func (db *Database) SaveUserProfile(userID int64, firstName, lastName, username string) error {
	if userID <= 0 {
		return errors.New("invalid user ID")
	}

	firstName = strings.TrimSpace(firstName)
	lastName = strings.TrimSpace(lastName)
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")

	ctx, cancel := db.ctx()
	defer cancel()

	setFields := bson.M{
		"updated_at": time.Now().Unix(),
	}
	if firstName != "" {
		setFields["first_name"] = firstName
	}
	if lastName != "" {
		setFields["last_name"] = lastName
	}
	if username != "" {
		setFields["username"] = username
	}

	_, err := db.userDB.UpdateOne(ctx,
		bson.M{"_id": userID},
		bson.M{
			"$set":         setFields,
			"$setOnInsert": bson.M{"_id": userID},
		},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return err
	}

	key := toKey(userID)
	cached, _ := db.userCache.Get(key)
	doc := &Users{
		ID:        userID,
		FirstName: firstName,
		LastName:  lastName,
		Username:  username,
		UpdatedAt: time.Now().Unix(),
	}
	if cached != nil {
		if doc.FirstName == "" {
			doc.FirstName = cached.FirstName
		}
		if doc.LastName == "" {
			doc.LastName = cached.LastName
		}
		if doc.Username == "" {
			doc.Username = cached.Username
		}
	}
	db.userCache.Set(key, doc)
	return nil
}

// GetUser retrieves a user's profile from the database if they have sent /start.
func (db *Database) GetUser(userID int64) (*Users, error) {
	if userID <= 0 {
		return nil, nil
	}
	key := toKey(userID)
	if cached, ok := db.userCache.Get(key); ok && cached != nil {
		return cached, nil
	}

	ctx, cancel := db.ctx()
	defer cancel()

	var user Users
	err := db.userDB.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	db.userCache.Set(key, &user)
	return &user, nil
}

// RemoveUser removes a user from the database and cache.
func (db *Database) RemoveUser(userID int64) error {
	ctx, cancel := db.ctx()
	defer cancel()

	_, err := db.userDB.DeleteOne(ctx, bson.M{"_id": userID})
	if err != nil {
		return err
	}

	db.userCache.Delete(toKey(userID))
	return nil
}

// IsUserExist checks if a user exists in the database.
func (db *Database) IsUserExist(userID int64) (bool, error) {
	u, err := db.GetUser(userID)
	return u != nil, err
}

// GetAllUsers retrieves a list of all user IDs from the database.
func (db *Database) GetAllUsers() ([]int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := db.userDB.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer func(cursor *mongo.Cursor, ctx context.Context) {
		_ = cursor.Close(ctx)
	}(cursor, ctx)

	var users []int64
	for cursor.Next(ctx) {
		var doc Users
		if err := cursor.Decode(&doc); err != nil {
			return nil, err
		}
		users = append(users, doc.ID)
		db.userCache.Set(toKey(doc.ID), &doc)
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return users, nil
}
