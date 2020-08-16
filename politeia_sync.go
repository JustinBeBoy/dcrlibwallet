package dcrlibwallet

import (
	"fmt"
	"time"

	"github.com/asdine/storm"
	"github.com/asdine/storm/q"
	www "github.com/decred/politeia/politeiawww/api/www/v1"
)

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
	updateInterval = 10 // 10 mins
)

func (p *Politeia) Sync(notificationListeners PoliteiaNotificationListeners) error {
	log.Info("Politeia sync: starting")

	p.notificationListeners = notificationListeners

	// fetch server policy if it's not been fetched
	if p.client.policy == nil {
		serverPolicy, err := p.client.serverPolicy()
		if err != nil {
			return err
		}
		p.client.policy = &serverPolicy
	}

	var savedTokens []string
	err := p.db.Select(q.True()).Each(new(Proposal), func(record interface{}) error {
		p := record.(*Proposal)
		savedTokens = append(savedTokens, p.CensorshipRecord.Token)
		return nil
	})
	if err != nil && err != storm.ErrNotFound {
		return fmt.Errorf("error loading saved proposals: %s", err.Error())
	}

	// fetch remote token inventory
	log.Info("Politeia sync: fetching token inventory")
	tokenInventory, err := p.client.tokenInventory()
	if err != nil {
		return err
	}

	err = p.fetchAllUnfetchedProposals(tokenInventory, savedTokens, false)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(updateInterval * time.Minute)
	p.syncQuitChan = make(chan struct{})

	go func() {
		for {
			select {
			case <-ticker.C:
				log.Info("Politeia sync: checking for proposal updates")
				p.checkForUpdates()
			case <-p.syncQuitChan:
				ticker.Stop()
				return
			}
		}
	}()
	<-p.syncQuitChan

	return nil
}

func (p *Politeia) fetchAllUnfetchedProposals(tokenInventory *TokenInventory, savedTokens []string, notify bool) error {
	preProposals := diff(tokenInventory.Pre, savedTokens)
	activeProposals := diff(tokenInventory.Active, savedTokens)
	approvedProposals := diff(tokenInventory.Approved, savedTokens)
	rejectedProposals := diff(tokenInventory.Rejected, savedTokens)
	abandonedProposals := diff(tokenInventory.Abandoned, savedTokens)

	totalNumProposalsToFetch := len(preProposals) + len(activeProposals) + len(approvedProposals) + len(rejectedProposals) + len(abandonedProposals)
	if totalNumProposalsToFetch > 0 {
		log.Infof("Politeia sync: fetching %d new proposals", totalNumProposalsToFetch)
	} else {
		log.Infof("Politeia sync: no new proposals found. Checking again in %d minutes", updateInterval)
		return nil
	}

	err := p.syncBatchProposals(PreProposals, preProposals, notify)
	if err != nil {
		return err
	}

	err = p.syncBatchProposals(ActiveProposals, activeProposals, notify)
	if err != nil {
		return err
	}

	err = p.syncBatchProposals(ApprovedProposals, approvedProposals, notify)
	if err != nil {
		return err
	}

	err = p.syncBatchProposals(RejectedProposals, rejectedProposals, notify)
	if err != nil {
		return err
	}

	err = p.syncBatchProposals(AbandonedProposals, abandonedProposals, notify)
	if err != nil {
		return err
	}

	return nil
}

func (p *Politeia) syncBatchProposals(category ProposalCategory, proposalsInventory []string, notify bool) error {
	for {
		if len(proposalsInventory) == 0 {
			break
		}

		var batch []string

		limit := p.client.policy.ProposalListPageSize
		if len(proposalsInventory) <= p.client.policy.ProposalListPageSize {
			limit = len(proposalsInventory)
		}

		batch, proposalsInventory = proposalsInventory[:limit], proposalsInventory[limit:]

		batchTokens := &Tokens{batch}
		proposals, err := p.client.batchProposals(batchTokens)
		if err != nil {
			return err
		}

		votesSummaries, err := p.client.batchVoteSummary(batchTokens)
		if err != nil {
			return err
		}

		for i := range proposals {
			proposals[i].Category = category
			if voteSummary, ok := votesSummaries.Summaries[proposals[i].CensorshipRecord.Token]; ok {
				proposals[i].VoteSummary = voteSummary
			}

			err = p.db.Save(&proposals[i])
			if err != nil {
				return fmt.Errorf("error saving new proposal: %s", err.Error())
			}

			if notify {
				p.onNewProposal(&proposals[i])
			}
		}
	}

	return nil
}

func (p *Politeia) checkForUpdates() error {
	var proposals []Proposal
	err := p.db.All(&proposals)
	if err != nil && err != storm.ErrNotFound {
		return err
	}

	err = p.handleVoteStatusChange(proposals)
	if err != nil {
		return err
	}

	err = p.handleNewProposals(proposals)
	if err != nil {
		return err
	}

	return nil
}

func (p *Politeia) handleVoteStatusChange(proposals []Proposal) error {
	votesStatus, err := p.client.votesStatus()
	if err != nil {
		return err
	}

	for i := range proposals {
		for k := range votesStatus {
			if proposals[i].CensorshipRecord.Token == votesStatus[k].Token && proposals[i].VoteStatus.Status != votesStatus[k].Status {
				proposals[i].VoteStatus = votesStatus[k]

				if proposals[i].VoteStatus.Status == int(www.PropVoteStatusStarted) {
					defer p.onVoteStarted(&proposals[i])
				} else if proposals[i].VoteStatus.Status == int(www.PropVoteStatusFinished) {
					defer p.onVoteFinished(&proposals[k])
				}

				err := p.db.UpdateField(&Proposal{ID: proposals[i].ID}, "VoteStatus", votesStatus[k])
				if err != nil {
					return fmt.Errorf("error handling changed vote status: %s", err.Error())
				}
			}
		}
	}

	return nil
}

func (p *Politeia) handleNewProposals(proposals []Proposal) error {
	loadedTokens := make([]string, len(proposals))
	for i := range proposals {
		loadedTokens[i] = proposals[i].CensorshipRecord.Token
	}

	tokenInventory, err := p.client.tokenInventory()
	if err != nil {
		return err
	}

	return p.fetchAllUnfetchedProposals(tokenInventory, loadedTokens, true)
}

func (p *Politeia) onVoteStarted(proposal *Proposal) {
	log.Info("Politeia sync: proposal vote status updated")
	p.notificationListeners.OnVoteStarted(proposal)
}

func (p *Politeia) onVoteFinished(proposal *Proposal) {
	log.Info("Politeia sync: proposal vote status updated")
	p.notificationListeners.OnVoteFinished(proposal)
}

func (p *Politeia) onNewProposal(proposal *Proposal) {
	log.Infof("Politeia sync: found new proposal %s", proposal.CensorshipRecord.Token)
	p.onNewProposal(proposal)
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
