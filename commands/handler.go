package commands

import (
	"log"
	"missgirlything/services"

	"github.com/bwmarrin/discordgo"
)

type CommandHandler struct {
	gifCommand     *GifCommand
	rankingCommand *RankingCommand
	commands       []*discordgo.ApplicationCommand
}

func NewCommandHandler(db *services.DBService, rankingService *services.RankingService) *CommandHandler {
	handler := &CommandHandler{
		gifCommand:     NewGifCommand(db),
		rankingCommand: NewRankingCommand(rankingService),
	}

	handler.commands = []*discordgo.ApplicationCommand{
		{
			Name:        "ping",
			Description: "Replies with pong!",
		},
		{
			Name:        "setgif",
			Description: "Set a GIF for another user (not yourself)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionUser,
					Name:        "user",
					Description: "The user to set the GIF for",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "gif_url",
					Description: "The GIF URL",
					Required:    true,
				},
			},
		},
		{
			Name:        "ranking",
			Description: "Display the weekly offensive language ranking",
		},
	}

	return handler
}

func (h *CommandHandler) RegisterCommands(session *discordgo.Session) error {
	// Get all guilds the bot is in
	guilds := session.State.Guilds
	
	for _, guild := range guilds {
		for _, cmd := range h.commands {
			_, err := session.ApplicationCommandCreate(session.State.User.ID, guild.ID, cmd)
			if err != nil {
				log.Printf("Error registering command %s in guild %s: %v", cmd.Name, guild.ID, err)
				continue
			}
			log.Printf("Registered command '%s' in guild: %s", cmd.Name, guild.Name)
		}
	}
	
	return nil
}

func (h *CommandHandler) HandleSlashCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Printf("Slash command received: %s", i.ApplicationCommandData().Name)
	switch i.ApplicationCommandData().Name {
	case "ping":
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "pong",
			},
		})
	case "setgif":
		h.gifCommand.HandleSlash(s, i)
	case "ranking":
		h.rankingCommand.HandleSlash(s, i)
	}
}
