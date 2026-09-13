package services

import (
	"sync"
	"testing"
	"time"
)

func TestAddVoteStoresBallot(t *testing.T) {
	svc := NewPollService()
	p := svc.CreatePoll("Q?", []string{"A", "B"}, "author", "chan", 0)
	if err := p.AddVote("u1", "user1", []int{8, 3}); err != nil {
		t.Fatalf("AddVote: %v", err)
	}
	if got := p.VoterCount(); got != 1 {
		t.Fatalf("VoterCount = %d, want 1", got)
	}
	// Revote remplace le bulletin.
	if err := p.AddVote("u1", "user1", []int{5, 5}); err != nil {
		t.Fatalf("revote: %v", err)
	}
	if got := p.VoterCount(); got != 1 {
		t.Fatalf("VoterCount après revote = %d, want 1", got)
	}
}

func TestAddVoteValidation(t *testing.T) {
	svc := NewPollService()
	p := svc.CreatePoll("Q?", []string{"A", "B"}, "author", "chan", 0)
	if err := p.AddVote("u1", "x", []int{5}); err == nil {
		t.Fatal("mauvais nombre de notes accepté")
	}
	if err := p.AddVote("u1", "x", []int{5, 11}); err == nil {
		t.Fatal("note 11 acceptée")
	}
	if err := p.AddVote("u1", "x", []int{5, -1}); err == nil {
		t.Fatal("note -1 acceptée")
	}
}

func TestCloseOnceIdempotent(t *testing.T) {
	svc := NewPollService()
	p := svc.CreatePoll("Q?", []string{"A", "B"}, "author", "chan", 0)
	_ = p.AddVote("u1", "x", []int{9, 1})
	if _, _, ok := svc.CloseOnce(p.ID); !ok {
		t.Fatal("première clôture refusée")
	}
	if _, _, ok := svc.CloseOnce(p.ID); ok {
		t.Fatal("double clôture acceptée (risque de double publication)")
	}
	if err := p.AddVote("u2", "y", []int{1, 9}); err == nil {
		t.Fatal("vote après clôture accepté")
	}
}

func TestComputeResultCondorcetWinner(t *testing.T) {
	svc := NewPollService()
	p := svc.CreatePoll("Q?", []string{"Dune", "Oppy", "Barbie"}, "a", "c", 0)
	for _, b := range [][]int{{9, 6, 4}, {8, 7, 5}, {10, 3, 2}} {
		if err := p.AddVote("u"+string(rune('0'+b[0])), "x", b); err != nil {
			t.Fatal(err)
		}
	}
	res := p.ComputeResult()
	if !res.HasCondorcet || res.CondorcetWinner != 0 {
		t.Fatalf("vainqueur attendu Dune, got %+v", res)
	}
	if res.Options[0].Name != "Dune" || res.Options[0].Copeland != 2 {
		t.Fatalf("classement inattendu: %+v", res.Options[0])
	}
}

func TestComputeResultCycle(t *testing.T) {
	svc := NewPollService()
	p := svc.CreatePoll("Q?", []string{"A", "B", "C"}, "a", "c", 0)
	_ = p.AddVote("u1", "x", []int{3, 2, 1})
	_ = p.AddVote("u2", "x", []int{1, 3, 2})
	_ = p.AddVote("u3", "x", []int{2, 1, 3})
	res := p.ComputeResult()
	if res.HasCondorcet {
		t.Fatal("vainqueur de Condorcet détecté dans un cycle")
	}
}

func TestConcurrentVotes(t *testing.T) {
	svc := NewPollService()
	p := svc.CreatePoll("Q?", []string{"A", "B"}, "author", "chan", 0)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = p.AddVote(string(rune('a'+i%26))+string(rune('0'+i/26)), "x", []int{i % 11, (i * 3) % 11})
			_ = p.VoterCount()
			_ = p.ComputeResult()
		}(i)
	}
	wg.Wait()
}

func TestStepperAdvanceSkipsUnmetCondition(t *testing.T) {
	polls := NewPollService()
	svc := NewStepperService(polls)
	st := svc.CreateStepper("T", "a", "c", time.Minute, []StepDef{
		{Question: "Q1", Options: []string{"Oui", "Non"}},
		{Question: "Q2", Options: []string{"Minecraft", "Enshrouded"}},
		{Question: "Q4", Options: []string{"Skyblock", "Modpack"}, Cond: StepCondition{HasCond: true, CondStep: 1, CondValue: "Minecraft"}},
	})
	// Enshrouded gagne l'étape 2 -> l'étape 3 doit être sautée, fin directe.
	_ = st.Steps[0].AddVote("u1", "x", []int{10, 0})
	_, _, _ = polls.CloseOnce(st.Steps[0].ID)
	next, finished := svc.Advance(st.ID)
	if finished || next != st.Steps[1] {
		t.Fatalf("étape 2 attendue, got next=%v finished=%v", next, finished)
	}
	_ = st.Steps[1].AddVote("u1", "x", []int{0, 10})
	_, _, _ = polls.CloseOnce(st.Steps[1].ID)
	next, finished = svc.Advance(st.ID)
	if !finished || next != nil {
		t.Fatalf("fin attendue avec étape sautée, got next=%v finished=%v", next, finished)
	}
	if !st.Steps[2].IsSkipped() {
		t.Fatal("l'étape conditionnelle aurait dû être sautée")
	}
}

func TestStepperCancelStopsAdvance(t *testing.T) {
	polls := NewPollService()
	svc := NewStepperService(polls)
	st := svc.CreateStepper("T", "a", "c", time.Minute, []StepDef{
		{Question: "Q1", Options: []string{"Oui", "Non"}},
		{Question: "Q2", Options: []string{"A", "B"}},
	})
	if !svc.Cancel(st.ID) {
		t.Fatal("annulation refusée")
	}
	_, _, _ = polls.CloseOnce(st.Steps[0].ID)
	if next, finished := svc.Advance(st.ID); next != nil || finished {
		t.Fatalf("après annulation: next=%v finished=%v, want (nil,false)", next, finished)
	}
}
