package services

import (
	"encoding/json"
	"log"
	"missgirlything/types"
	"os"
	"sync"
	"time"
)

type DBService struct {
	mu       sync.RWMutex
	filePath string
	db       *types.Database
}

func NewDBService(filePath string) *DBService {
	service := &DBService{
		filePath: filePath,
		db: &types.Database{
			Users: make(map[string]*types.UserData),
		},
	}
	
	// Try to load existing file
	if err := service.Load(); err != nil {
		log.Printf("Error loading database: %v", err)
	}
	
	// Create the file if it doesn't exist
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		log.Printf("Creating new database file: %s", filePath)
		if err := service.Save(); err != nil {
			log.Printf("Error creating database file: %v", err)
		}
	}
	
	return service
}

func (s *DBService) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return json.Unmarshal(data, s.db)
}

func (s *DBService) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := json.MarshalIndent(s.db, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}

func (s *DBService) GetUser(userID string) *types.UserData {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if user, exists := s.db.Users[userID]; exists {
		return user
	}
	return nil
}

func (s *DBService) CreateOrUpdateUser(userID string, updateFn func(*types.UserData)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, exists := s.db.Users[userID]
	if !exists {
		user = &types.UserData{
			UserID:         userID,
			LastMessages:   []string{},
			GifURL:         "",
			OffensiveCount: 0,
			LastWeekReset:  time.Now(),
		}
		s.db.Users[userID] = user
	}

	updateFn(user)
	
	// Save without acquiring lock (we already have it)
	data, err := json.MarshalIndent(s.db, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}

func (s *DBService) GetAllUsers() []*types.UserData {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]*types.UserData, 0, len(s.db.Users))
	for _, user := range s.db.Users {
		users = append(users, user)
	}
	return users
}

func (s *DBService) ResetWeeklyStats() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, user := range s.db.Users {
		user.WeeklyOffensive = 0
		user.LastWeekReset = time.Now()
	}

	return s.Save()
}
