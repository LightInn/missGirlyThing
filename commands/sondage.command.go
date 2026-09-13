package commands

import (
	"bytes"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"missgirlything/services"

	"github.com/bwmarrin/discordgo"
)

// SondageCommand lance un sondage au scrutin de Condorcet par notation (0-10).
// Vote aveugle : chacun note les options sans voir les notes des autres.
// Dépouillement : préférences par paire dérivées des notes + méthode Copeland,
// vainqueur de Condorcet détecté, graphique PNG joint à l'embed de résultats.
type SondageCommand struct {
	polls *services.PollService
	// OnStepClosed est appelé après chaque clôture publiée (nil = rien).
	// Utilisé par le stepper pour enchaîner l'étape suivante.
	OnStepClosed func(s *discordgo.Session, poll *services.Poll)
}

func NewSondageCommand(polls *services.PollService) *SondageCommand {
	return &SondageCommand{polls: polls}
}

func SondageDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "sondage",
		Description: "Lance un sondage Condorcet (vote aveugle par notes 0-10 + graphique)",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "question",
				Description: "La question du sondage",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "choix",
				Description: "Options séparées par ;  (2 à 5, ex: Dune ; Oppenheimer ; Barbie)",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "duree_minutes",
				Description: "Durée avant clôture auto (défaut 60, max 1440)",
				Required:    false,
			},
		},
	}
}

const (
	maxOptions     = 5
	minOptions     = 2
	modalPrefix    = "sondage_vote_"
	btnVotePrefix  = "sondage_voter_"
	btnResPrefix   = "sondage_resultats_"
	btnClosePrefix = "sondage_cloturer_"
)

var optionDots = []string{"🟥", "🟧", "🟨", "🟩", "🟦"}

// ---------- slash ----------

func (c *SondageCommand) HandleSlash(s *discordgo.Session, i *discordgo.InteractionCreate) {
	opts := map[string]*discordgo.ApplicationCommandInteractionDataOption{}
	for _, o := range i.ApplicationCommandData().Options {
		opts[o.Name] = o
	}
	question := strings.TrimSpace(opts["question"].StringValue())
	choixRaw := opts["choix"].StringValue()
	duree := int64(60)
	if o, ok := opts["duree_minutes"]; ok {
		duree = o.IntValue()
	}
	if duree < 1 {
		duree = 1
	}
	if duree > 1440 {
		duree = 1440
	}

	options, errMsg := parseChoices(choixRaw)
	if errMsg != "" {
		ephemeral(s, i, "❌ "+errMsg)
		return
	}
	if strings.TrimSpace(question) == "" {
		ephemeral(s, i, "❌ La question ne peut pas être vide.")
		return
	}

	authorID := interactionAuthorID(i)
	poll := c.polls.CreatePoll(question, options, authorID, i.ChannelID, time.Duration(duree)*time.Minute)

	embed := c.pollEmbed(poll)
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: c.pollComponents(poll.ID),
		},
	}); err != nil {
		log.Printf("sondage: erreur réponse interaction: %v", err)
		return
	}

	// Récupère le message posté pour pouvoir l'éditer (compteur, clôture).
	msg, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{})
	if err != nil {
		log.Printf("sondage: impossible de récupérer le message: %v", err)
	} else {
		poll.MessageID = msg.ID
	}

	// Clôture automatique.
	pollID := poll.ID
	c.armAutoClose(s, pollID, time.Duration(duree)*time.Minute)
}

// PostPollMessage poste un sondage déjà créé dans un salon (étapes du stepper),
// mémorise le message et arme la clôture auto. Les boutons/modales existants
// fonctionnent tels quels car tout est indexé par l'ID du sondage.
func (c *SondageCommand) PostPollMessage(s *discordgo.Session, channelID string, poll *services.Poll, duration time.Duration) error {
	if duration > 0 {
		poll.ClosesAt = time.Now().Add(duration)
	}
	msg, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embeds:     []*discordgo.MessageEmbed{c.pollEmbed(poll)},
		Components: c.pollComponents(poll.ID),
	})
	if err != nil {
		return err
	}
	poll.MessageID = msg.ID
	c.armAutoClose(s, poll.ID, duration)
	return nil
}

func (c *SondageCommand) armAutoClose(s *discordgo.Session, pollID string, duration time.Duration) {
	if duration <= 0 {
		return
	}
	time.AfterFunc(duration, func() {
		p := c.polls.Get(pollID)
		if p == nil || p.IsClosed() {
			return
		}
		c.closeAndPublish(s, pollID, "", true)
	})
}

func parseChoices(raw string) ([]string, string) {
	sep := ";"
	if !strings.Contains(raw, ";") {
		if strings.Contains(raw, "|") {
			sep = "|"
		} else if strings.Contains(raw, "\n") {
			sep = "\n"
		} else if strings.Count(raw, ",") >= 1 {
			sep = ","
		}
	}
	parts := strings.Split(raw, sep)
	options := []string{}
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t == "" {
			continue
		}
		if len([]rune(t)) > 80 {
			t = string([]rune(t)[:80])
		}
		options = append(options, t)
	}
	if len(options) < minOptions {
		return nil, fmt.Sprintf("il faut au moins %d options séparées par « ; » (ex : Dune ; Oppenheimer ; Barbie).", minOptions)
	}
	if len(options) > maxOptions {
		return nil, fmt.Sprintf("maximum %d options (limite des formulaires Discord).", maxOptions)
	}
	seen := map[string]bool{}
	for _, o := range options {
		k := strings.ToLower(o)
		if seen[k] {
			return nil, "les options doivent être différentes."
		}
		seen[k] = true
	}
	return options, ""
}

// ---------- components (boutons) ----------

// HandleComponent traite les boutons du sondage. Retourne true si géré.
func (c *SondageCommand) HandleComponent(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	customID := i.MessageComponentData().CustomID
	var pollID string
	var kind string
	switch {
	case strings.HasPrefix(customID, btnVotePrefix):
		pollID, kind = strings.TrimPrefix(customID, btnVotePrefix), "vote"
	case strings.HasPrefix(customID, btnResPrefix):
		pollID, kind = strings.TrimPrefix(customID, btnResPrefix), "resultats"
	case strings.HasPrefix(customID, btnClosePrefix):
		pollID, kind = strings.TrimPrefix(customID, btnClosePrefix), "cloturer"
	default:
		return false
	}

	poll := c.polls.Get(pollID)
	if poll == nil {
		ephemeral(s, i, "❌ Sondage introuvable (le bot a peut-être redémarré).")
		return true
	}

	switch kind {
	case "vote":
		c.openVoteModal(s, i, poll)
	case "resultats":
		c.handleResultats(s, i, poll)
	case "cloturer":
		c.handleCloturer(s, i, poll)
	}
	return true
}

func (c *SondageCommand) openVoteModal(s *discordgo.Session, i *discordgo.InteractionCreate, poll *services.Poll) {
	if poll.IsClosed() {
		ephemeral(s, i, "🔒 Ce sondage est clôturé, les votes sont fermés.")
		return
	}
	rows := make([]discordgo.MessageComponent, 0, len(poll.Options))
	for idx, name := range poll.Options {
		label := name
		if len([]rune(label)) > 28 {
			label = string([]rune(label)[:28]) + "…"
		}
		rows = append(rows, discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.TextInput{
					CustomID:    fmt.Sprintf("note_%d", idx),
					Label:       fmt.Sprintf("%s (0-10)", label),
					Style:       discordgo.TextInputShort,
					Required:    true,
					Placeholder: "0 (contre) à 10 (adoré)",
					MinLength:   1,
					MaxLength:   2,
				},
			},
		})
	}
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID:   modalPrefix + poll.ID,
			Title:      "Voter — notes 0 à 10",
			Components: rows,
		},
	})
	if err != nil {
		log.Printf("sondage: erreur ouverture modale: %v", err)
	}
}

func (c *SondageCommand) handleResultats(s *discordgo.Session, i *discordgo.InteractionCreate, poll *services.Poll) {
	if !poll.IsClosed() {
		ephemeral(s, i, fmt.Sprintf("🙈 Vote aveugle en cours : **%d** vote(s). Les résultats restent cachés jusqu'à la clôture.", poll.VoterCount()))
		return
	}
	res := poll.ComputeResult()
	summary := resultSummary(poll, res)
	ephemeral(s, i, summary)
}

func (c *SondageCommand) handleCloturer(s *discordgo.Session, i *discordgo.InteractionCreate, poll *services.Poll) {
	if poll.IsClosed() {
		ephemeral(s, i, "🔒 Ce sondage est déjà clôturé.")
		return
	}
	if !canClose(s, i, poll) {
		ephemeral(s, i, "❌ Seul l'auteur du sondage (ou un modo « Gérer les messages ») peut le clôturer.")
		return
	}
	ephemeral(s, i, "🔒 Clôture en cours, calcul Condorcet + graphique…")
	c.closeAndPublish(s, poll.ID, interactionAuthorID(i), false)
}

// ---------- modale (dépôt du vote) ----------

// HandleModal traite le retour du formulaire de vote. Retourne true si géré.
func (c *SondageCommand) HandleModal(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	customID := i.ModalSubmitData().CustomID
	if !strings.HasPrefix(customID, modalPrefix) {
		return false
	}
	pollID := strings.TrimPrefix(customID, modalPrefix)
	poll := c.polls.Get(pollID)
	if poll == nil {
		ephemeral(s, i, "❌ Sondage introuvable (le bot a peut-être redémarré).")
		return true
	}
	if poll.IsClosed() {
		ephemeral(s, i, "🔒 Trop tard, le sondage vient d'être clôturé.")
		return true
	}

	scores := make([]int, len(poll.Options))
	data := i.ModalSubmitData()
	pos := 0
	for _, row := range data.Components {
		var inner []discordgo.MessageComponent
		switch ar := row.(type) {
		case *discordgo.ActionsRow:
			inner = ar.Components
		case discordgo.ActionsRow:
			inner = ar.Components
		default:
			continue
		}
		for _, cmp := range inner {
			var value string
			switch ti := cmp.(type) {
			case *discordgo.TextInput:
				value = ti.Value
			case discordgo.TextInput:
				value = ti.Value
			default:
				continue
			}
			if pos >= len(scores) {
				break
			}
			v, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || v < 0 || v > 10 {
				ephemeral(s, i, fmt.Sprintf("❌ Note invalide pour « %s » : mets un nombre entre 0 et 10.", poll.Options[pos]))
				return true
			}
			scores[pos] = v
			pos++
		}
	}
	if pos != len(scores) {
		ephemeral(s, i, "❌ Il faut noter toutes les options (0 à 10).")
		return true
	}

	username := interactionAuthorName(s, i)
	if err := poll.AddVote(interactionAuthorID(i), username, scores); err != nil {
		ephemeral(s, i, "❌ "+err.Error())
		return true
	}

	ephemeral(s, i, fmt.Sprintf("✅ Vote enregistré (aveugle 🙈) : **%d** vote(s) au total. Tu peux revoter pour modifier.", poll.VoterCount()))
	c.refreshPollMessage(s, poll)
	return true
}

// ---------- clôture + publication ----------

func (c *SondageCommand) closeAndPublish(s *discordgo.Session, pollID, closerID string, auto bool) {
	poll, res, ok := c.polls.CloseOnce(pollID)
	if !ok {
		return // déjà clôturé (ex : manuel + timer auto) : ne rien republier
	}
	if poll == nil || res == nil {
		return
	}
	embed := c.resultEmbed(poll, res, closerID, auto)

	chart, err := services.RenderPollChart(res)
	if err != nil {
		log.Printf("sondage: erreur graphique: %v", err)
	}

	// Édite le message d'origine : résultats + graphique, boutons supprimés.
	if poll.MessageID != "" {
		if chart != nil {
			embed.Image = &discordgo.MessageEmbedImage{URL: "attachment://condorcet.png"}
			_, _ = s.ChannelMessageEditComplex(&discordgo.MessageEdit{
				ID:         poll.MessageID,
				Channel:    poll.ChannelID,
				Embeds:     &[]*discordgo.MessageEmbed{embed},
				Components: &[]discordgo.MessageComponent{},
				Files: []*discordgo.File{{
					Name:        "condorcet.png",
					ContentType: "image/png",
					Reader:      bytes.NewReader(chart),
				}},
			})
		} else {
			_, _ = s.ChannelMessageEditComplex(&discordgo.MessageEdit{
				ID:         poll.MessageID,
				Channel:    poll.ChannelID,
				Embeds:     &[]*discordgo.MessageEmbed{embed},
				Components: &[]discordgo.MessageComponent{},
			})
		}
	} else if chart != nil {
		_, _ = s.ChannelMessageSendComplex(poll.ChannelID, &discordgo.MessageSend{
			Embeds: []*discordgo.MessageEmbed{embed},
			Files: []*discordgo.File{{
				Name:        "condorcet.png",
				ContentType: "image/png",
				Reader:      bytes.NewReader(chart),
			}},
		})
	} else {
		_, _ = s.ChannelMessageSendEmbed(poll.ChannelID, embed)
	}

	// Enchaînement stepper éventuel.
	if c.OnStepClosed != nil && poll.StepperID != "" {
		c.OnStepClosed(s, poll)
	}
}

func (c *SondageCommand) refreshPollMessage(s *discordgo.Session, poll *services.Poll) {
	if poll.MessageID == "" || poll.IsClosed() {
		return
	}
	embed := c.pollEmbed(poll)
	_, _ = s.ChannelMessageEditComplex(&discordgo.MessageEdit{
		ID:         poll.MessageID,
		Channel:    poll.ChannelID,
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &[]discordgo.MessageComponent{discordgo.ActionsRow{Components: c.pollButtons(poll.ID)}},
	})
}

// ---------- embeds ----------

func (c *SondageCommand) pollComponents(pollID string) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: c.pollButtons(pollID)},
	}
}

func (c *SondageCommand) pollButtons(pollID string) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.Button{Label: "Voter", Style: discordgo.PrimaryButton, Emoji: &discordgo.ComponentEmoji{Name: "🗳️"}, CustomID: btnVotePrefix + pollID},
		discordgo.Button{Label: "Résultats", Style: discordgo.SecondaryButton, Emoji: &discordgo.ComponentEmoji{Name: "📊"}, CustomID: btnResPrefix + pollID},
		discordgo.Button{Label: "Clôturer", Style: discordgo.DangerButton, Emoji: &discordgo.ComponentEmoji{Name: "🔒"}, CustomID: btnClosePrefix + pollID},
	}
}

func (c *SondageCommand) pollEmbed(poll *services.Poll) *discordgo.MessageEmbed {
	lines := make([]string, len(poll.Options))
	for idx, name := range poll.Options {
		lines[idx] = fmt.Sprintf("%s **%d.** %s", optionDots[idx%len(optionDots)], idx+1, name)
	}
	title := "🗳️ " + poll.Question
	footer := "Vote aveugle 🙈 — les notes restent cachées jusqu'à la clôture."
	if poll.StepperID != "" && poll.StepTotal > 0 {
		title = fmt.Sprintf("🗳️ Étape %d/%d — %s", poll.StepIndex, poll.StepTotal, poll.Question)
	}
	if !poll.ClosesAt.IsZero() {
		footer += fmt.Sprintf(" Clôture auto <t:%d:R>.", poll.ClosesAt.Unix())
	}
	return &discordgo.MessageEmbed{
		Title:       title,
		Description: strings.Join(lines, "\n") + "\n\n_Clique sur **Voter** et donne une note de 0 à 10 à chaque option._",
		Color:       0x5865F2,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "🙈 Votes", Value: fmt.Sprintf("**%d** vote(s)", poll.VoterCount()), Inline: true},
			{Name: "⚖️ Méthode", Value: "Condorcet + Copeland", Inline: true},
		},
		Footer: &discordgo.MessageEmbedFooter{Text: footer},
		Timestamp: poll.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func (c *SondageCommand) resultEmbed(poll *services.Poll, res *services.PollResult, closerID string, auto bool) *discordgo.MessageEmbed {
	head := ""
	if res.TotalVoters == 0 {
		head = "_Aucun vote._ 😿"
	} else if res.HasCondorcet {
		head = fmt.Sprintf("🏆 **Vainqueur de Condorcet : %s** (bat chaque option en duel) — %d vote(s).",
			poll.Options[res.CondorcetWinner], res.TotalVoters)
	} else {
		head = fmt.Sprintf("⚖️ **Pas de vainqueur de Condorcet strict** (cycle) — vainqueur Copeland : **%s** — %d vote(s).",
			res.Options[0].Name, res.TotalVoters)
	}

	fields := make([]*discordgo.MessageEmbedField, 0, len(res.Options))
	for _, o := range res.Options {
		medal := medalFor(o.Rank)
		fields = append(fields, &discordgo.MessageEmbedField{
			Name: fmt.Sprintf("%s %s %s", medal, optionDots[o.Index%len(optionDots)], o.Name),
			Value: fmt.Sprintf("%s `%.1f/10`  •  Copeland **%+d** (%dV %dD %dN)\n%s",
				medal, o.Average, o.Copeland, o.Wins, o.Losses, o.Ties, bar(o.Average)),
			Inline: false,
		})
	}

	matrix := pairwiseMatrix(poll, res)
	footer := "Scrutin Condorcet par notes 0-10 • vote aveugle 🙈 • départage : Copeland puis moyenne."
	if auto {
		footer += " Clôture automatique."
	} else if closerID != "" {
		footer += fmt.Sprintf(" Clôturé par <@%s>.", closerID)
	}

	return &discordgo.MessageEmbed{
		Title:       "📊 " + poll.Question,
		Description: head + "\n\n**Duels (votants préférant la ligne à la colonne) :**\n" + matrix,
		Color:       0x57F287,
		Fields:      fields,
		Footer:      &discordgo.MessageEmbedFooter{Text: footer},
		Timestamp:   time.Now().Format("2006-01-02T15:04:05Z07:00"),
	}
}

func resultSummary(poll *services.Poll, res *services.PollResult) string {
	if res.TotalVoters == 0 {
		return "📊 Sondage clôturé : aucun vote. 😿"
	}
	var b strings.Builder
	if res.HasCondorcet {
		fmt.Fprintf(&b, "🏆 **%s** (vainqueur de Condorcet) — %d vote(s)\n", poll.Options[res.CondorcetWinner], res.TotalVoters)
	} else {
		fmt.Fprintf(&b, "⚖️ Pas de vainqueur strict — top Copeland : **%s** — %d vote(s)\n", res.Options[0].Name, res.TotalVoters)
	}
	for _, o := range res.Options {
		fmt.Fprintf(&b, "%s %s : %.1f/10 (Copeland %+d)\n", medalFor(o.Rank), o.Name, o.Average, o.Copeland)
	}
	return b.String()
}

func pairwiseMatrix(poll *services.Poll, res *services.PollResult) string {
	var b strings.Builder
	b.WriteString("```\n      ")
	for i := range poll.Options {
		fmt.Fprintf(&b, "%4d ", i+1)
	}
	b.WriteString("\n")
	for i := range poll.Options {
		fmt.Fprintf(&b, " %2d.  ", i+1)
		for j := range poll.Options {
			if i == j {
				b.WriteString("  —  ")
			} else {
				fmt.Fprintf(&b, "%4d ", res.Pairwise[i][j])
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("```")
	return b.String()
}

// bar rend une barre unicode 10 blocs proportionnelle à la moyenne /10.
func bar(avg float64) string {
	full := int(avg + 0.5)
	if full < 0 {
		full = 0
	}
	if full > 10 {
		full = 10
	}
	return "`" + strings.Repeat("█", full) + strings.Repeat("░", 10-full) + "`"
}

func medalFor(rank int) string {
	switch rank {
	case 1:
		return "🥇"
	case 2:
		return "🥈"
	case 3:
		return "🥉"
	default:
		return fmt.Sprintf("`%d.`", rank)
	}
}

// ---------- helpers interactions ----------

func ephemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func interactionAuthorID(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

func interactionAuthorName(s *discordgo.Session, i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.Username
	}
	if i.User != nil {
		return i.User.Username
	}
	return "?"
}

func canClose(s *discordgo.Session, i *discordgo.InteractionCreate, poll *services.Poll) bool {
	if interactionAuthorID(i) == poll.AuthorID {
		return true
	}
	if i.Member != nil && i.Member.Permissions&discordgo.PermissionManageMessages != 0 {
		return true
	}
	return false
}
