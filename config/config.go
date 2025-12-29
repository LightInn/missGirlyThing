package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	DiscordToken      string
	WordList          []string
	GifDisplaySeconds int
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	gifSeconds, err := strconv.Atoi(getEnv("GIF_DISPLAY_SECONDS", "3"))
	if err != nil {
		gifSeconds = 3
	}

	wordListStr := getEnv("WORD_LIST", "")
	wordList := []string{}
	if wordListStr != "" {
		wordList = strings.Split(wordListStr, ",")
		for i := range wordList {
			wordList[i] = strings.TrimSpace(strings.ToLower(wordList[i]))
		}
	}

	return &Config{
		DiscordToken:      getEnv("DISCORD_TOKEN", ""),
		WordList:          wordList,
		GifDisplaySeconds: gifSeconds,
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
