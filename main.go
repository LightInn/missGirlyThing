package main

import (
	"log"
	"missgirlything/commands"
	"missgirlything/config"
	"missgirlything/services"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
)

func main() {
	// Load configuration
	cfg := config.Load()

	if cfg.DiscordToken == "" {
		log.Fatal("DISCORD_TOKEN environment variable is required")
	}

	// Initialize services
	dbService := services.NewDBService("data/save.json")
	messageService := services.NewMessageService(dbService, cfg)
	rankingService := services.NewRankingService(dbService)

	// Create Discord session
	discord, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		log.Fatalf("Error creating Discord session: %v", err)
	}

	// Initialize command handler with Discord session
	commandHandler := commands.NewCommandHandler(dbService, rankingService)

	// Track last active channel
	var lastChannelID string
	var lastChannelMutex sync.Mutex

	// Register slash command handlers
	discord.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		lastChannelMutex.Lock()
		lastChannelID = i.ChannelID
		lastChannelMutex.Unlock()
		commandHandler.HandleSlashCommand(s, i)
	})

	// Register message handlers
	discord.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.Bot {
			return
		}
		lastChannelMutex.Lock()
		lastChannelID = m.ChannelID
		lastChannelMutex.Unlock()
		log.Printf("Message received from %s: %s", m.Author.Username, m.Content)
		messageService.HandleMessage(s, m)
	})

	discord.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Bot is ready! Logged in as %s", r.User.Username)
		log.Printf("Bot is in %d guilds", len(r.Guilds))
	})

	// Set intents
	discord.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent | discordgo.IntentsGuilds

	// Open connection
	err = discord.Open()
	if err != nil {
		log.Fatalf("Error opening Discord connection: %v", err)
	}
	defer discord.Close()

	// Register slash commands
	log.Println("Registering slash commands...")
	err = commandHandler.RegisterCommands(discord)
	if err != nil {
		log.Fatalf("Error registering commands: %v", err)
	}
	log.Println("Slash commands registered successfully!")

	log.Println("Bot is now running. Press CTRL+C to exit.")

	// Start weekly ranking checker (in production environment only)
	// You would typically get the channel ID from config or database
	// go rankingService.StartWeeklyRanking(discord, "YOUR_CHANNEL_ID")

	// Wait for interrupt signal
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	log.Println("Shutting down gracefully...")

	// Send goodbye message to last active channel
	lastChannelMutex.Lock()
	if lastChannelID != "" {
		log.Printf("Sending goodbye message to channel %s", lastChannelID)
		_, err := discord.ChannelMessageSend(lastChannelID, "aurevoir les amis, je vais dormir :3")
		if err != nil {
			log.Printf("Error sending goodbye message: %v", err)
		} else {
			// Wait a bit to ensure message is sent
			time.Sleep(1 * time.Second)
		}
	}
	lastChannelMutex.Unlock()
}
