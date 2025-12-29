package commands

import (
	"fmt"
	"log"
	"missgirlything/services"
	"missgirlything/types"

	"github.com/bwmarrin/discordgo"
)

type GifCommand struct {
	dbService *services.DBService
}

func NewGifCommand(db *services.DBService) *GifCommand {
	return &GifCommand{
		dbService: db,
	}
}

func (c *GifCommand) HandleSlash(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Printf("Processing setgif command")

	// Respond immediately to Discord (ephemeral so only command user sees it)
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Printf("Error responding to interaction: %v", err)
		return
	}

	options := i.ApplicationCommandData().Options

	optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(options))
	for _, opt := range options {
		optionMap[opt.Name] = opt
	}

	targetUser := optionMap["user"].UserValue(s)
	gifURL := optionMap["gif_url"].StringValue()

	log.Printf("Setting GIF for user %s: %s", targetUser.Username, gifURL)

	// Check if trying to set own GIF
	var authorID string
	if i.Member != nil {
		authorID = i.Member.User.ID
	} else {
		authorID = i.User.ID
	}

	if targetUser.ID == authorID {
		content := "❌ You cannot set a GIF for yourself!"
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &content,
		})
		return
	}

	err = c.dbService.CreateOrUpdateUser(targetUser.ID, func(user *types.UserData) {
		user.GifURL = gifURL
	})

	if err != nil {
		log.Printf("Error setting GIF: %v", err)
		content := "❌ Error setting GIF!"
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &content,
		})
		return
	}

	content := fmt.Sprintf("✅ GIF set for <@%s>! It will appear when they send messages.", targetUser.ID)
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &content,
	})
	log.Printf("GIF set successfully")
}
