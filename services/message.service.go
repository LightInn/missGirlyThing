package services

import (
	"log"
	"missgirlything/config"
	"missgirlything/types"
	"regexp"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

type MessageService struct {
	dbService *DBService
	config    *config.Config
}

func NewMessageService(db *DBService, cfg *config.Config) *MessageService {
	return &MessageService{
		dbService: db,
		config:    cfg,
	}
}

func (s *MessageService) HandleMessage(session *discordgo.Session, message *discordgo.MessageCreate) {
	if message.Author.Bot {
		return
	}

	userID := message.Author.ID

	// Check if bot is mentioned
	s.checkBotMention(session, message)

	// Display GIF if user has one
	go s.displayGif(session, message.ChannelID, userID)

	// Check for "xD" response
	s.checkXD(session, message)

	// Track message
	err := s.dbService.CreateOrUpdateUser(userID, func(user *types.UserData) {
		user.LastMessages = append(user.LastMessages, message.Content)
		if len(user.LastMessages) > 3 {
			user.LastMessages = user.LastMessages[len(user.LastMessages)-3:]
		}

		// Check for offensive words
		contentLower := strings.ToLower(message.Content)
		for _, word := range s.config.WordList {
			if strings.Contains(contentLower, word) {
				user.OffensiveCount++
				user.WeeklyOffensive++
				log.Printf("Offensive word detected from user %s", userID)
				break
			}
		}
	})

	if err != nil {
		log.Printf("Error updating user data: %v", err)
		return
	}

	// Check for MDR variants
	s.checkMDR(session, message)
}

func (s *MessageService) displayGif(session *discordgo.Session, channelID, userID string) {
	user := s.dbService.GetUser(userID)
	if user == nil || user.GifURL == "" {
		return
	}

	msg, err := session.ChannelMessageSend(channelID, user.GifURL)
	if err != nil {
		log.Printf("Error sending GIF: %v", err)
		return
	}

	time.Sleep(time.Duration(s.config.GifDisplaySeconds) * time.Second)
	session.ChannelMessageDelete(channelID, msg.ID)
}

func (s *MessageService) checkBotMention(session *discordgo.Session, message *discordgo.MessageCreate) {
	// Check if the bot is mentioned in the message
	for _, mention := range message.Mentions {
		if mention.ID == session.State.User.ID {
			log.Printf("Bot was mentioned! Replying with UwU")
			_, err := session.ChannelMessageSendReply(message.ChannelID, "UwU", message.Reference())
			if err != nil {
				log.Printf("Error sending UwU reply: %v", err)
			}
			return
		}
	}
}

func (s *MessageService) checkXD(session *discordgo.Session, message *discordgo.MessageCreate) {
	// Check for "xD" (case insensitive)
	xdRegex := regexp.MustCompile(`(?i)\bxd\b`)
	
	if xdRegex.MatchString(message.Content) {
		log.Printf("xD detected! Replying to message")
		_, err := session.ChannelMessageSendReply(message.ChannelID, "ca c'est un jolie smiley !", message.Reference())
		if err != nil {
			log.Printf("Error sending xD reply: %v", err)
		}
	}
}

func (s *MessageService) checkMDR(session *discordgo.Session, message *discordgo.MessageCreate) {
	user := s.dbService.GetUser(message.Author.ID)
	if user == nil || len(user.LastMessages) == 0 {
		return
	}

	log.Printf("Checking MDR for user %s with messages: %v", message.Author.ID, user.LastMessages)

	// Concatenate last 3 messages into one string
	combinedMessages := strings.Join(user.LastMessages, " ")
	
	// Check with regex for MDR variants on the concatenated string
	// Matches: mdr, MDR, m d r, m dr, md r, mdrrr, mmmdr, etc.
	// This allows detection across multiple messages like:
	// Message 1: "lol m"
	// Message 2: "d"
	// Message 3: "r haha"
	mdrRegex := regexp.MustCompile(`(?i)\bm+\s*d+\s*r+\b`)
	
	if mdrRegex.MatchString(combinedMessages) {
		log.Printf("MDR variant detected in combined messages! Replying with 'tg'")
		_, err := session.ChannelMessageSendReply(message.ChannelID, "tg", message.Reference())
		if err != nil {
			log.Printf("Error sending 'tg' reply: %v", err)
		}
		
		// Clear the message history to avoid triggering again
		err = s.dbService.CreateOrUpdateUser(message.Author.ID, func(user *types.UserData) {
			user.LastMessages = []string{}
		})
		if err != nil {
			log.Printf("Error clearing message history: %v", err)
		}
		return
	}

	log.Printf("No MDR variant found")
}
