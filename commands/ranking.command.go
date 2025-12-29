package commands

import (
	"missgirlything/services"

	"github.com/bwmarrin/discordgo"
)

type RankingCommand struct {
	rankingService *services.RankingService
}

func NewRankingCommand(rankingService *services.RankingService) *RankingCommand {
	return &RankingCommand{
		rankingService: rankingService,
	}
}

func (c *RankingCommand) HandleSlash(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Defer the response since ranking might take time
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	ranking := c.rankingService.GetWeeklyRankingText(s)

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &ranking,
	})
}
