package commands

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"missgirlything/services"

	"github.com/bwmarrin/discordgo"
)

// StepperCommand enchaîne plusieurs votes Condorcet étape par étape.
// Exemple : dispo → jeu (Enshrouded/Minecraft) → timing → si Minecraft :
// skyblock ou modpack. Chaque étape est un vote aveugle 0-10 classique ;
// l'étape suivante se poste à la clôture de la précédente, sauf condition
// non remplie (étape sautée ⏭️) ou annulation. Un récap clôt la session.
type StepperCommand struct {
	svc     *services.StepperService
	sondage *SondageCommand
}

func NewStepperCommand(svc *services.StepperService, sondage *SondageCommand) *StepperCommand {
	return &StepperCommand{svc: svc, sondage: sondage}
}

const maxSteps = 5

const stepperCancelPrefix = "stepper_cancel_"

func StepperDefinition() *discordgo.ApplicationCommand {
	opts := []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "titre",
			Description: "Titre du vote en plusieurs étapes",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionInteger,
			Name:        "duree_minutes",
			Description: "Durée de chaque étape (défaut 60, max 1440)",
			Required:    false,
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "question1",
			Description: "Question de l'étape 1",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "choix1",
			Description: "Options de l'étape 1 séparées par ;",
			Required:    true,
		},
	}
	for i := 2; i <= maxSteps; i++ {
		opts = append(opts,
			&discordgo.ApplicationCommandOption{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        fmt.Sprintf("question%d", i),
				Description: fmt.Sprintf("Question de l'étape %d (vide = fin)", i),
				Required:    false,
			},
			&discordgo.ApplicationCommandOption{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        fmt.Sprintf("choix%d", i),
				Description: fmt.Sprintf("Options de l'étape %d séparées par ;", i),
				Required:    false,
			},
			&discordgo.ApplicationCommandOption{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        fmt.Sprintf("si%d", i),
				Description: fmt.Sprintf("Condition de l'étape %d (ex : 2=Minecraft). Vide = toujours.", i),
				Required:    false,
			},
		)
	}
	return &discordgo.ApplicationCommand{
		Name:        "sondage_stepper",
		Description: "Enchaîne plusieurs votes Condorcet étape par étape (avec conditions)",
		Options:     opts,
	}
}

// ---------- création ----------

func (c *StepperCommand) HandleSlash(s *discordgo.Session, i *discordgo.InteractionCreate) {
	raw := map[string]*discordgo.ApplicationCommandInteractionDataOption{}
	for _, o := range i.ApplicationCommandData().Options {
		raw[o.Name] = o
	}
	str := func(name string) string {
		if o, ok := raw[name]; ok {
			return strings.TrimSpace(o.StringValue())
		}
		return ""
	}

	title := str("titre")
	if title == "" {
		ephemeral(s, i, "❌ Le titre ne peut pas être vide.")
		return
	}
	duree := int64(60)
	if o, ok := raw["duree_minutes"]; ok {
		duree = o.IntValue()
	}
	if duree < 1 {
		duree = 1
	}
	if duree > 1440 {
		duree = 1440
	}
	duration := time.Duration(duree) * time.Minute

	defs := []services.StepDef{}
	for step := 1; step <= maxSteps; step++ {
		q := str(fmt.Sprintf("question%d", step))
		ch := ""
		if o, ok := raw[fmt.Sprintf("choix%d", step)]; ok {
			ch = o.StringValue()
		}
		si := str(fmt.Sprintf("si%d", step))
		if q == "" {
			if ch != "" || si != "" {
				ephemeral(s, i, fmt.Sprintf("❌ L'étape %d a des choix/condition sans question.", step))
				return
			}
			continue // étapes suivantes éventuellement définies ? non : on stoppe
		}
		// Question présente mais trou dans la numérotation ?
		if len(defs)+1 != step {
			ephemeral(s, i, fmt.Sprintf("❌ Les étapes doivent se suivre sans trou (problème à l'étape %d).", step))
			return
		}
		options, errMsg := parseChoices(ch)
		if errMsg != "" {
			ephemeral(s, i, fmt.Sprintf("❌ Étape %d : %s", step, errMsg))
			return
		}
		def := services.StepDef{Question: q, Options: options}
		if si != "" {
			cond, errMsg := parseStepCond(si, step)
			if errMsg != "" {
				ephemeral(s, i, fmt.Sprintf("❌ Étape %d : %s", step, errMsg))
				return
			}
			def.Cond = cond
		}
		defs = append(defs, def)
	}
	if len(defs) == 0 {
		ephemeral(s, i, "❌ Aucune étape définie.")
		return
	}

	authorID := interactionAuthorID(i)
	st := c.svc.CreateStepper(title, authorID, i.ChannelID, duration, defs)

	// Message d'intro : sommaire + bouton d'annulation.
	intro := c.introEmbed(st)
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{intro},
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.Button{Label: "Annuler la suite", Style: discordgo.DangerButton, Emoji: &discordgo.ComponentEmoji{Name: "⏹️"}, CustomID: stepperCancelPrefix + st.ID},
				}},
			},
		},
	}); err != nil {
		log.Printf("stepper: erreur réponse interaction: %v", err)
		return
	}
	if msg, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{}); err == nil {
		st.IntroMessageID = msg.ID
	}

	// Poste la première étape.
	if err := c.sondage.PostPollMessage(s, st.ChannelID, st.Steps[0], duration); err != nil {
		log.Printf("stepper: impossible de poster l'étape 1: %v", err)
		ephemeral(s, i, "❌ Impossible de poster la première étape.")
	}
}

// parseStepCond lit "2=Minecraft" : étape de référence 1-based < step,
// valeur = nom d'option ou numéro d'option.
func parseStepCond(raw string, step int) (services.StepCondition, string) {
	parts := strings.SplitN(raw, "=", 2)
	if len(parts) != 2 {
		return services.StepCondition{}, "condition invalide, format attendu : 2=Minecraft (n° d'étape = option gagnante)."
	}
	n, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || n < 1 || n >= step {
		return services.StepCondition{}, fmt.Sprintf("la condition doit référencer une étape précédente (1 à %d).", step-1)
	}
	val := strings.TrimSpace(parts[1])
	if val == "" {
		return services.StepCondition{}, "condition invalide : option gagnante vide (ex : 2=Minecraft)."
	}
	return services.StepCondition{HasCond: true, CondStep: n - 1, CondValue: val}, ""
}

// ---------- enchaînement ----------

// AfterStepClosed est branché sur SondageCommand.OnStepClosed : poste l'étape
// suivante (ou le récap) après publication des résultats de l'étape fermée.
func (c *StepperCommand) AfterStepClosed(s *discordgo.Session, poll *services.Poll) {
	if poll.StepperID == "" {
		return
	}
	st := c.svc.Get(poll.StepperID)
	if st == nil {
		return
	}
	next, finished := c.svc.Advance(st.ID)
	switch {
	case next != nil:
		if err := c.sondage.PostPollMessage(s, next.ChannelID, next, st.Duration); err != nil {
			log.Printf("stepper: impossible de poster l'étape %d: %v", next.StepIndex, err)
		}
	case finished:
		c.postRecap(s, st)
	}
	// Sinon : stepper annulé/terminé → ne rien faire.
}

// ---------- annulation ----------

// HandleComponent traite le bouton d'annulation. Retourne true si géré.
func (c *StepperCommand) HandleComponent(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	customID := i.MessageComponentData().CustomID
	if !strings.HasPrefix(customID, stepperCancelPrefix) {
		return false
	}
	st := c.svc.Get(strings.TrimPrefix(customID, stepperCancelPrefix))
	if st == nil {
		ephemeral(s, i, "❌ Vote introuvable (le bot a peut-être redémarré).")
		return true
	}
	if interactionAuthorID(i) != st.AuthorID &&
		!(i.Member != nil && i.Member.Permissions&discordgo.PermissionManageMessages != 0) {
		ephemeral(s, i, "❌ Seul l'auteur (ou un modo « Gérer les messages ») peut annuler la suite.")
		return true
	}
	if !c.svc.Cancel(st.ID) {
		ephemeral(s, i, "ℹ️ Ce vote est déjà terminé ou annulé.")
		return true
	}
	// Retire le bouton d'annulation de l'intro.
	if st.IntroMessageID != "" {
		_, _ = s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			ID:         st.IntroMessageID,
			Channel:    st.ChannelID,
			Components: &[]discordgo.MessageComponent{},
		})
	}
	ephemeral(s, i, "⏹️ Suite annulée. L'étape en cours reste votable mais n'enchaînera plus rien.")
	return true
}

// ---------- embeds ----------

func (c *StepperCommand) introEmbed(st *services.Stepper) *discordgo.MessageEmbed {
	lines := make([]string, len(st.Defs))
	for idx, d := range st.Defs {
		line := fmt.Sprintf("**%d.** %s — _%s_", idx+1, d.Question, strings.Join(d.Options, ", "))
		if label := services.CondLabel(d); label != "" {
			line += fmt.Sprintf("\n↳ ⏭️ *%s*", label)
		}
		lines[idx] = line
	}
	return &discordgo.MessageEmbed{
		Title:       "🧭 " + st.Title,
		Description: "Vote en plusieurs étapes : chaque étape se lance à la clôture de la précédente.\n\n" + strings.Join(lines, "\n"),
		Color:       0x5865F2,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "⚖️ Méthode", Value: "Condorcet par notes 0-10, vote aveugle 🙈", Inline: true},
			{Name: "⏱️ Durée", Value: fmt.Sprintf("%d min / étape", int(st.Duration.Minutes())), Inline: true},
		},
		Footer:    &discordgo.MessageEmbedFooter{Text: "Clôturer une étape lance la suivante • Annuler la suite stoppe l'enchaînement."},
		Timestamp: time.Now().Format("2006-01-02T15:04:05Z07:00"),
	}
}

func (c *StepperCommand) postRecap(s *discordgo.Session, st *services.Stepper) {
	fields := make([]*discordgo.MessageEmbedField, 0, len(st.Defs))
	for idx, d := range st.Defs {
		p := st.Steps[idx]
		name := fmt.Sprintf("Étape %d — %s", idx+1, d.Question)
		switch {
		case p.IsSkipped():
			fields = append(fields, &discordgo.MessageEmbedField{
				Name: "⏭️ " + name, Value: "_Ignorée (condition non remplie)._", Inline: false,
			})
		default:
			res := p.ComputeResult()
			if res.TotalVoters == 0 {
				fields = append(fields, &discordgo.MessageEmbedField{
					Name: "❔ " + name, Value: "_Aucun vote._", Inline: false,
				})
			} else {
				top := res.Options[0]
				fields = append(fields, &discordgo.MessageEmbedField{
					Name: fmt.Sprintf("✅ %s", name),
					Value: fmt.Sprintf("**%s** — `%.1f/10` (Copeland %+d, %d vote(s))",
						top.Name, top.Average, top.Copeland, res.TotalVoters),
					Inline: false,
				})
			}
		}
	}
	embed := &discordgo.MessageEmbed{
		Title:       "🏁 " + st.Title + " — récapitulatif",
		Description: "Les graphiques détaillés sont sur le message de chaque étape.",
		Color:       0xFACD50,
		Fields:      fields,
		Footer:      &discordgo.MessageEmbedFooter{Text: "Scrutin Condorcet par notes • vote aveugle 🙈"},
		Timestamp:   time.Now().Format("2006-01-02T15:04:05Z07:00"),
	}
	if _, err := s.ChannelMessageSendEmbed(st.ChannelID, embed); err != nil {
		log.Printf("stepper: impossible de poster le récap: %v", err)
	}
}
