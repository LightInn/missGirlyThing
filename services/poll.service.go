package services

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Poll est un sondage au scrutin de Condorcet par notation.
// Chaque votant attribue une note 0-10 à chaque option (vote aveugle :
// les notes des autres ne sont jamais révélées avant la clôture).
// Le dépouillement dérive de chaque bulletin des préférences par paire
// (note A > note B => A bat B chez ce votant), puis applique Copeland.
type Poll struct {
	ID        string
	Question  string
	Options   []string
	AuthorID  string
	ChannelID string
	MessageID string
	CreatedAt time.Time
	ClosesAt  time.Time
	Closed    bool

	mu         sync.RWMutex
	Votes      map[string][]int  // userID -> notes par option
	VoterNames map[string]string // userID -> pseudo (pour debug/logs, jamais affiché avant clôture)

	// Champs stepper (vides pour un sondage simple).
	StepperID string // ID du stepper parent, "" si sondage isolé
	StepIndex int    // numéro d'étape 1-based (affichage "Étape X/N")
	StepTotal int    // nombre total d'étapes
	Skipped   bool   // true si l'étape a été sautée (condition non remplie)
}

// OptionResult est le résultat calculé pour une option.
type OptionResult struct {
	Index    int
	Name     string
	Average  float64 // moyenne des notes (0-10)
	Wins     int     // duels par paire gagnés
	Losses   int     // duels perdus
	Ties     int     // égalités
	Copeland int     // Wins - Losses
	Rank     int     // 1 = premier
}

// PollResult est le dépouillement complet d'un sondage.
type PollResult struct {
	Options         []*OptionResult // triées par rang
	Pairwise        [][]int         // Pairwise[i][j] = nb de votants préférant i à j (strict)
	TiesPair        [][]int         // TiesPair[i][j] = nb de votants à égalité i == j
	TotalVoters     int
	CondorcetWinner int  // index de l'option gagnante, -1 si aucun vainqueur strict
	HasCondorcet    bool // true si un vainqueur de Condorcet strict existe
}

type PollService struct {
	mu    sync.RWMutex
	polls map[string]*Poll
}

func NewPollService() *PollService {
	return &PollService{polls: make(map[string]*Poll)}
}

func newPollID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// CreatePoll crée un sondage ouvert. duration <= 0 => pas d'expiration auto.
func (s *PollService) CreatePoll(question string, options []string, authorID, channelID string, duration time.Duration) *Poll {
	p := &Poll{
		ID:         newPollID(),
		Question:   question,
		Options:    options,
		AuthorID:   authorID,
		ChannelID:  channelID,
		CreatedAt:  time.Now(),
		Votes:      make(map[string][]int),
		VoterNames: make(map[string]string),
	}
	if duration > 0 {
		p.ClosesAt = p.CreatedAt.Add(duration)
	}
	s.mu.Lock()
	s.polls[p.ID] = p
	s.mu.Unlock()
	return p
}

func (s *PollService) Get(id string) *Poll {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.polls[id]
}

// AddVote enregistre (ou remplace) le bulletin d'un votant. Aveugle : aucune lecture partielle possible.
func (p *Poll) AddVote(userID, username string, scores []int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.Closed {
		return fmt.Errorf("sondage clôturé")
	}
	if len(scores) != len(p.Options) {
		return fmt.Errorf("il faut noter les %d options", len(p.Options))
	}
	for _, n := range scores {
		if n < 0 || n > 10 {
			return fmt.Errorf("les notes doivent être entre 0 et 10")
		}
	}
	cp := make([]int, len(scores))
	copy(cp, scores)
	p.Votes[userID] = cp
	p.VoterNames[userID] = username
	return nil
}

func (p *Poll) VoterCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.Votes)
}

func (p *Poll) IsOpen() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return !p.Closed
}

// IsClosed lecture thread-safe de l'état de clôture.
func (p *Poll) IsClosed() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Closed
}

// IsSkipped lecture thread-safe du flag "étape sautée".
func (p *Poll) IsSkipped() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Skipped
}

// Close clôture le sondage et retourne le dépouillement.
func (s *PollService) Close(id string) (*Poll, *PollResult) {
	p, res, _ := s.CloseOnce(id)
	return p, res
}

// CloseOnce clôture une seule fois : si le sondage est déjà clôturé
// (ex : clôture manuelle + timer auto en course), ok vaut false et rien
// n'est recalculé. Indispensable pour les steppers (anti double-enchaînement).
func (s *PollService) CloseOnce(id string) (*Poll, *PollResult, bool) {
	p := s.Get(id)
	if p == nil {
		return nil, nil, false
	}
	p.mu.Lock()
	if p.Closed {
		p.mu.Unlock()
		return p, nil, false
	}
	p.Closed = true
	p.mu.Unlock()
	return p, p.ComputeResult(), true
}

// ComputeResult dépouille selon Condorcet/Copeland + moyenne (départage et graphique).
func (p *Poll) ComputeResult() *PollResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	n := len(p.Options)
	// Copie des bulletins pour calcul hors verrou prolongé (on est déjà en RLock, calcul pur).
	ballots := make([][]int, 0, len(p.Votes))
	for _, v := range p.Votes {
		cp := make([]int, n)
		copy(cp, v)
		ballots = append(ballots, cp)
	}

	pairwise := make([][]int, n)
	ties := make([][]int, n)
	for i := range pairwise {
		pairwise[i] = make([]int, n)
		ties[i] = make([]int, n)
	}
	for _, b := range ballots {
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				if i == j {
					continue
				}
				if b[i] > b[j] {
					pairwise[i][j]++
				} else if b[i] == b[j] {
					ties[i][j]++
				}
			}
		}
	}

	results := make([]*OptionResult, n)
	for i := 0; i < n; i++ {
		wins, losses, eq := 0, 0, 0
		for j := 0; j < n; j++ {
			if i == j {
				continue
			}
			switch {
			case pairwise[i][j] > pairwise[j][i]:
				wins++
			case pairwise[i][j] < pairwise[j][i]:
				losses++
			default:
				eq++
			}
		}
		sum := 0
		for _, b := range ballots {
			sum += b[i]
		}
		avg := 0.0
		if len(ballots) > 0 {
			avg = float64(sum) / float64(len(ballots))
		}
		results[i] = &OptionResult{
			Index: i, Name: p.Options[i], Average: avg,
			Wins: wins, Losses: losses, Ties: eq, Copeland: wins - losses,
		}
	}

	// Vainqueur de Condorcet strict : bat chaque autre option en duel.
	winner := -1
	for i := 0; i < n; i++ {
		beatsAll := true
		for j := 0; j < n; j++ {
			if i == j {
				continue
			}
			if pairwise[i][j] <= pairwise[j][i] {
				beatsAll = false
				break
			}
		}
		if beatsAll {
			winner = i
			break
		}
	}

	// Classement : Copeland desc, puis moyenne desc, puis victoires desc, puis ordre.
	sort.Slice(results, func(a, b int) bool {
		if results[a].Copeland != results[b].Copeland {
			return results[a].Copeland > results[b].Copeland
		}
		if results[a].Average != results[b].Average {
			return results[a].Average > results[b].Average
		}
		if results[a].Wins != results[b].Wins {
			return results[a].Wins > results[b].Wins
		}
		return results[a].Index < results[b].Index
	})
	for i, r := range results {
		r.Rank = i + 1
	}

	return &PollResult{
		Options:         results,
		Pairwise:        pairwise,
		TiesPair:        ties,
		TotalVoters:     len(ballots),
		CondorcetWinner: winner,
		HasCondorcet:    winner >= 0,
	}
}
