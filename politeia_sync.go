package dcrlibwallet

import (
	//"fmt"
	//"time"

	"github.com/asdine/storm"
)

// 1. Fetch token inventory
// 2. Determine proposals that have not been saved to db and fetch them
// 3. loop through and check if any of the proposals have been edited
// 4. fetch all proposal files
// 5. start checking for new proposals or votes

type ProposalCategory int

const (
	AllProposals = iota + 1
	PreProposals
	ActiveProposals
	ApprovedProposals
	RejectedProposals
	AbandonedProposals
)

const (
	updateInterval = 5 // 5 mins
)

func (p *Politeia) Sync() error {
	// fetch all proposals from db
	var proposals []Proposal
	err := p.db.All(&proposals)
	if err != nil && err != storm.ErrNotFound {
		return err
	}

	var savedTokens []string
	for i := range proposals {
		savedTokens = append(savedTokens, proposals[i].CensorshipRecord.Token)
	}

	// fetch server policy if it's not been fetched
	if p.client.policy == nil {
		serverPolicy, err := p.client.serverPolicy()
		if err != nil {
			return err
		}
		p.client.policy = &serverPolicy
	}

	// fetch all token inventory
	tokenInventory, err := p.client.tokenInventory()
	if err != nil {
		return err
	}

	// TODO add this to the list of savedTokens
	_, err = p.fetchAllUnfetchedProposals(tokenInventory, savedTokens)
	if err != nil {
		return err
	}

	/**ticker := time.NewTicker(updateInterval * time.Minute)
	p.syncQuitChan = make(chan struct{})

	go func() {
		for {
			select {
			case <-ticker.C:
				p.checkForUpdates()
			case <-p.syncQuitChan:
				ticker.Stop()
				return
			}
		}
	}()
	<-p.syncQuitChan**/

	return nil
}

/**func (p *Politeia) checkForUpdates() error {
	// fetch all saved proposals first
	var proposals []Proposal
	err := p.db.All(&proposals)
	if err != nil && err != storm.ErrNotFound {
		return err
	}

	var savedTokens []string
	for i := range proposals {
		savedTokens = append(savedTokens, proposals[i].CensorshipRecord.Token)
	}

	// fetch latest token inventory
	tokenInventory, err := p.client.tokenInventory()
	if err != nil {
		return err
	}

	// fetch all

	return nil
}**/

// TODO return all new proposals proposals
func (p *Politeia) fetchAllUnfetchedProposals(tokenInventory *TokenInventory, savedTokens []string) ([]Proposal, error) {
	newPreProposals, err := p.syncBatchProposals(PreProposals, diff(tokenInventory.Pre, savedTokens))
	if err != nil {
		return nil, err
	}

	newActiveProposals, err := p.syncBatchProposals(ActiveProposals, diff(tokenInventory.Active, savedTokens))
	if err != nil {
		return nil, err
	}

	newApprovedProposals, err := p.syncBatchProposals(ApprovedProposals, diff(tokenInventory.Approved, savedTokens))
	if err != nil {
		return nil, err
	}

	newRejectedProposals, err := p.syncBatchProposals(RejectedProposals, diff(tokenInventory.Rejected, savedTokens))
	if err != nil {
		return nil, err
	}

	newAbandonedProposals, err := p.syncBatchProposals(AbandonedProposals, diff(tokenInventory.Abandoned, savedTokens))
	if err != nil {
		return nil, err
	}

	newProposals := append(newPreProposals, newActiveProposals...)
	newProposals = append(newProposals, newApprovedProposals...)
	newProposals = append(newProposals, newRejectedProposals...)
	newProposals = append(newProposals, newAbandonedProposals...)

	return newProposals, nil
}

func (p *Politeia) syncBatchProposals(category ProposalCategory, proposalsInventory []string) ([]Proposal, error) {
	var newProposals []Proposal

	for {
		if len(proposalsInventory) == 0 {
			break
		}

		var batch []string
		var limit int
		if len(proposalsInventory) > p.client.policy.ProposalListPageSize {
			limit = p.client.policy.ProposalListPageSize
		} else {
			limit = len(proposalsInventory)
		}
		batch, proposalsInventory = proposalsInventory[:limit], proposalsInventory[limit:]

		batchTokens := &Tokens{batch}
		proposals, err := p.client.batchProposals(batchTokens)
		if err != nil {
			return nil, err
		}

		votesSummaries, err := p.client.batchVoteSummary(batchTokens)
		if err != nil {
			return nil, err
		}

		for i := range proposals {
			proposals[i].Category = category
			if voteSummary, ok := votesSummaries.Summaries[proposals[i].CensorshipRecord.Token]; ok {
				proposals[i].VoteSummary = voteSummary
			}

			newProposals = append(newProposals, proposals...)

			// TODO perform this in a transaction
			err = p.db.Save(&proposals[i])
			if err != nil {
				return nil, err
			}
		}
	}

	return newProposals, nil
}

func diff(tokenInventory, savedTokens []string) []string {
	var diff []string

	for i := range tokenInventory {
		exists := false

		for k := range savedTokens {
			if savedTokens[k] == tokenInventory[i] {
				exists = true
				break
			}
		}

		if !exists {
			diff = append(diff, tokenInventory[i])
		}
	}

	return diff
}
