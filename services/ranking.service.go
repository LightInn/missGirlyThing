package services

import (
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/bwmarrin/discordgo"
)

type RankingService struct {
	dbService *DBService
}

func NewRankingService(db *DBService) *RankingService {
	return &RankingService{
		dbService: db,
	}
}

func (s *RankingService) StartWeeklyRanking(session *discordgo.Session, channelID string) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		if now.Weekday() == time.Sunday && now.Hour() == 20 {
			s.DisplayWeeklyRanking(session, channelID)
		}
	}
}

func (s *RankingService) GetWeeklyRankingText(session *discordgo.Session) string {
	users := s.dbService.GetAllUsers()

	// Sort by weekly offensive count
	sort.Slice(users, func(i, j int) bool {
		return users[i].WeeklyOffensive > users[j].WeeklyOffensive
	})

	ranking := "🏆 **Weekly Offensive Ranking** 🏆\n\n"
	
	for i, user := range users {
		if i >= 10 || user.WeeklyOffensive == 0 {
			break
		}

		userInfo, err := session.User(user.UserID)
		username := user.UserID
		if err == nil {
			username = userInfo.Username
		}

		medal := ""
		switch i {
		case 0:
			medal = "🥇"
		case 1:
			medal = "🥈"
		case 2:
			medal = "🥉"
		default:
			medal = fmt.Sprintf("%d.", i+1)
		}

		ranking += fmt.Sprintf("%s **%s** - %d offensive words\n", medal, username, user.WeeklyOffensive)
	}

	if len(users) == 0 || (len(users) > 0 && users[0].WeeklyOffensive == 0) {
		ranking += "No offensive words this week! 👼"
	}

	return ranking
}

func (s *RankingService) DisplayWeeklyRanking(session *discordgo.Session, channelID string) {
	ranking := s.GetWeeklyRankingText(session)
	session.ChannelMessageSend(channelID, ranking)

	// Reset weekly stats
	if err := s.dbService.ResetWeeklyStats(); err != nil {
		log.Printf("Error resetting weekly stats: %v", err)
	}
}
