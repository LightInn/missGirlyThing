package services

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
)

// StepCondition : une étape peut ne se lancer que si le gagnant (rang 1)
// d'une étape précédente correspond à CondValue.
// CondValue accepte un nom d'option (insensible à la casse, ex "Minecraft")
// ou un numéro d'option 1-based (ex "1").
type StepCondition struct {
	HasCond   bool
	CondStep  int // 0-based, toujours < index de l'étape porteuse
	CondValue string
}

type StepDef struct {
	Question string
	Options  []string // ignoré pour un appel dispo
	Type     string   // StepTypeCondorcet (défaut) ou StepTypeDispo
	Cond     StepCondition
}

// Stepper enchaîne plusieurs sondages Condorcet étape par étape :
// l'étape N+1 se poste quand l'étape N est clôturée (manuellement ou auto),
// sauf condition non remplie (étape sautée) ou annulation.
type Stepper struct {
	ID             string
	Title          string
	AuthorID       string
	ChannelID      string
	IntroMessageID string
	Duration       time.Duration // durée de chaque étape (<=0 = manuel uniquement)
	Defs           []StepDef
	Steps          []*Poll // même longueur que Defs, sondages créés d'avance
	Current        int     // index de l'étape en cours (len(Defs) = terminé)
	Cancelled      bool
	mu             sync.RWMutex
}

type StepperService struct {
	mu       sync.RWMutex
	polls    *PollService
	steppers map[string]*Stepper
}

func NewStepperService(polls *PollService) *StepperService {
	return &StepperService{
		polls:    polls,
		steppers: make(map[string]*Stepper),
	}
}

// CreateStepper crée le stepper et pré-crée les sondages (non postés).
// Les étapes sont postées une par une via Current.
func (s *StepperService) CreateStepper(title, authorID, channelID string, duration time.Duration, defs []StepDef) *Stepper {
	st := &Stepper{
		ID:        newPollID(),
		Title:     title,
		AuthorID:  authorID,
		ChannelID: channelID,
		Duration:  duration,
		Defs:      defs,
		Current:   0,
	}
	for i, d := range defs {
		kind := d.Type
		if kind == "" {
			kind = StepTypeCondorcet
		}
		p := s.polls.CreatePoll(d.Question, d.Options, authorID, channelID, 0)
		p.StepperID = st.ID
		p.StepIndex = i + 1
		p.StepTotal = len(defs)
		p.IsDispo = kind == StepTypeDispo
		st.Steps = append(st.Steps, p)
	}
	s.mu.Lock()
	s.steppers[st.ID] = st
	s.mu.Unlock()
	return st
}

func (s *StepperService) Get(id string) *Stepper {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.steppers[id]
}

// Cancel annule la suite du stepper (l'étape en cours reste votable
// mais n'enchaînera plus rien). Retourne false si introuvable/terminé.
func (s *StepperService) Cancel(id string) bool {
	st := s.Get(id)
	if st == nil {
		return false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.Cancelled || st.Current >= len(st.Defs) {
		return false
	}
	st.Cancelled = true
	return true
}

func (s *StepperService) IsCancelled(st *Stepper) bool {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Cancelled
}

func (s *StepperService) IsDone(st *Stepper) bool {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Current >= len(st.Defs)
}

// SkipStep marque une étape comme sautée avec la raison (affichée au récap).
func (s *StepperService) SkipStep(p *Poll, reason string) {
	p.mu.Lock()
	p.Closed = true
	p.Skipped = true
	p.SkipReason = reason
	p.mu.Unlock()
}

// Advance fait progresser le stepper après la clôture de l'étape courante.
// - next != nil : le sondage à poster pour l'étape suivante.
// - finished == true : plus rien à poster, l'appelant publie le récapitulatif.
// - (nil, false) : stepper annulé, terminé ou introuvable → ne rien faire.
// Les étapes dont la condition n'est pas remplie sont marquées sautées.
func (s *StepperService) Advance(id string) (next *Poll, finished bool) {
	st := s.Get(id)
	if st == nil {
		return nil, false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.Cancelled || st.Current >= len(st.Defs) {
		return nil, false
	}
	for idx := st.Current + 1; idx < len(st.Defs); idx++ {
		ok, reason := s.CondMet(st, st.Defs[idx])
		if ok {
			st.Current = idx
			return st.Steps[idx], false
		}
		log.Printf("stepper %s: étape %d ignorée (%s)", st.ID, idx+1, reason)
		s.SkipStep(st.Steps[idx], reason)
	}
	st.Current = len(st.Defs)
	return nil, true
}

// CondMet évalue si l'étape doit se lancer. Sans condition : toujours.
// L'étape de référence doit être clôturée, non sautée, avec au moins un vote,
// et son gagnant (rang 1) doit correspondre à CondValue.
func (s *StepperService) CondMet(st *Stepper, def StepDef) (bool, string) {
	if !def.Cond.HasCond {
		return true, ""
	}
	if def.Cond.CondStep < 0 || def.Cond.CondStep >= len(st.Steps) {
		return false, "condition invalide"
	}
	ref := st.Steps[def.Cond.CondStep]
	if ref.IsSkipped() || !ref.IsClosed() {
		return false, "l'étape de référence a été ignorée"
	}
	if ref.IsDispo {
		return false, "l'étape de référence est un appel (pas de gagnant Condorcet)"
	}
	res := ref.ComputeResult()
	if res.TotalVoters == 0 {
		return false, "aucun vote à l'étape de référence"
	}
	top := res.Options[0]
	want := strings.TrimSpace(def.Cond.CondValue)
	if n, err := strconv.Atoi(want); err == nil {
		if top.Index+1 == n {
			return true, ""
		}
		return false, fmt.Sprintf("l'option n°%d n'a pas gagné l'étape %d (%s a gagné)",
			n, def.Cond.CondStep+1, ref.Options[top.Index])
	}
	if strings.EqualFold(strings.TrimSpace(top.Name), want) {
		return true, ""
	}
	// Repli : préfixe unique insensible à la casse (ex "Mine" → "Minecraft").
	// Évite les faux rejets sur une quasi-coquille ; ambiguïté = refusé.
	if len([]rune(want)) >= 2 {
		matched := ""
		ambiguous := false
		for _, o := range res.Options {
			if startsFold(o.Name, want) {
				if matched != "" {
					ambiguous = true
					break
				}
				matched = o.Name
			}
		}
		if !ambiguous && matched != "" && strings.EqualFold(matched, top.Name) {
			return true, ""
		}
	}
	return false, fmt.Sprintf("« %s » n'a pas gagné l'étape %d (%s a gagné)",
		want, def.Cond.CondStep+1, top.Name)
}

func startsFold(s, prefix string) bool {
	return strings.HasPrefix(strings.ToLower(s), strings.ToLower(prefix))
}

// CondLabel décrit la condition pour l'affichage (message d'intro).
func CondLabel(def StepDef) string {
	if !def.Cond.HasCond {
		return ""
	}
	return fmt.Sprintf("seulement si « %s » gagne l'étape %d",
		strings.TrimSpace(def.Cond.CondValue), def.Cond.CondStep+1)
}
