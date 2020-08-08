package dcrlibwallet

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/asdine/storm"
	"github.com/asdine/storm/q"
)

type Politeia struct {
	client *client
}

func NewPoliteia(rootDir string) (*Politeia, error) {
	var err error
	configDB, err = storm.Open(filepath.Join(rootDir, politeiaConfigDbName))
	if err != nil {
		return nil, fmt.Errorf("error opening politeia database: %s", err.Error())
	}

	err = configDB.Init(&PoliteiaConfig{})
	if err != nil {
		return nil, fmt.Errorf("error initializing politeia config database: %s", err.Error())
	}

	config := &PoliteiaConfig{}

	var shouldFetchVersion bool
	err = configDB.Select(q.Eq("ID", 1)).First(config)
	if err != nil {
		if err == storm.ErrNotFound {
			shouldFetchVersion = true
		} else {
			return nil, fmt.Errorf("error loading politeia config: %s", err.Error())
		}
	}

	client, err := newPoliteiaClient(config)
	if err != nil {
		return nil, err
	}

	if shouldFetchVersion {
		config.ID = 1
		configDB.Save(config)

		_, err = client.version()
		if err != nil {
			return nil, err
		}
	}

	return &Politeia{
		client: client,
	}, nil
}

func (p *Politeia) Shutdown() {
	configDB.Close()
}

func (p *Politeia) result(id string, res interface{}) (string, error) {
	resByte, err := json.Marshal(res)
	if err != nil {
		return "", fmt.Errorf("error marshalling %s result: %s", id, err.Error())
	}

	return string(resByte), nil
}

// GetTokenInventory fetches the censorship record tokens of all proposals in the inventory
func (p *Politeia) GetTokenInventory() (string, error) {
	tokenInventory, err := p.client.tokenInventory()
	if err != nil {
		return "", err
	}

	return p.result("GetTokenInventory", tokenInventory)
}

//GetBatchPreProposals retrieves the proposal details for a list of pre-vote proposals using an array of csfr as params
func (p *Politeia) GetBatchPreProposals() (string, error) {
	tokenInventory, err := p.client.tokenInventory()
	if err != nil {
		return "", err
	}

	var prevotesproposals *Tokens
	if len(tokenInventory.Pre) > 20 {
		prevotesproposals = &Tokens{tokenInventory.Pre[:20]}
	} else {
		prevotesproposals = &Tokens{tokenInventory.Pre}
	}

	proposals, err := p.client.batchProposals(prevotesproposals)
	if err != nil {
		return "", err
	}

	votesStatus, err := p.client.batchVoteSummary(prevotesproposals)
	if err != nil {
		return "", err
	}

	for i := range proposals {
		for j := range votesStatus {
			voteStatus := votesStatus[j]
			proposals[i].VoteStatus = voteStatus
		}
	}

	return p.result("GetBatchPreProposals", proposals)
}

//GetBatchActiveProposals retrieves the proposal details for a list of pre-vote proposals using an array of csfr as params
func (p *Politeia) GetBatchActiveProposals() (string, error) {
	tokenInventory, err := p.client.tokenInventory()
	if err != nil {
		return "", err
	}

	var activeproposals *Tokens
	if len(tokenInventory.Active) > 20 {
		activeproposals = &Tokens{tokenInventory.Active[:20]}
	} else {
		activeproposals = &Tokens{tokenInventory.Active}
	}

	proposals, err := p.client.batchProposals(activeproposals)
	if err != nil {
		return "", err
	}

	votesStatus, err := p.client.batchVoteSummary(activeproposals)
	if err != nil {
		return "", err
	}

	for i := range proposals {
		for j := range votesStatus {
			voteStatus := votesStatus[j]
			proposals[i].VoteStatus = voteStatus
		}
	}

	return p.result("GetBatchActiveProposals", proposals)
}

//GetBatchApprovedProposals retrieves the proposal details for a list of pre-vote proposals using an array of csfr as params
func (p *Politeia) GetBatchApprovedProposals() (string, error) {
	tokenInventory, err := p.client.tokenInventory()
	if err != nil {
		return "", err
	}

	var approvedproposals *Tokens
	if len(tokenInventory.Approved) > 20 {
		approvedproposals = &Tokens{tokenInventory.Approved[:20]}
	} else {
		approvedproposals = &Tokens{tokenInventory.Approved}
	}

	proposals, err := p.client.batchProposals(approvedproposals)
	if err != nil {
		return "", err
	}

	votesStatus, err := p.client.batchVoteSummary(approvedproposals)
	if err != nil {
		return "", err
	}

	for i := range proposals {
		for j := range votesStatus {
			voteStatus := votesStatus[j]
			proposals[i].VoteStatus = voteStatus
		}
	}

	return p.result("GetBatchApprovedProposals", proposals)
}

//GetBatchRejectedProposals retrieves the proposal details for a list of pre-vote proposals using an array of csfr as params
func (p *Politeia) GetBatchRejectedProposals() (string, error) {
	tokenInventory, err := p.client.tokenInventory()
	if err != nil {
		return "", err
	}

	var rejectedproposals *Tokens
	if len(tokenInventory.Rejected) > 20 {
		rejectedproposals = &Tokens{tokenInventory.Rejected[:20]}
	} else {
		rejectedproposals = &Tokens{tokenInventory.Rejected}
	}

	proposals, err := p.client.batchProposals(rejectedproposals)
	if err != nil {
		return "", err
	}

	votesStatus, err := p.client.batchVoteSummary(rejectedproposals)
	if err != nil {
		return "", err
	}

	for i := range proposals {
		for j := range votesStatus {
			voteStatus := votesStatus[j]
			proposals[i].VoteStatus = voteStatus
		}
	}

	return p.result("GetBatchRejectedProposals", proposals)
}

//GetBatchAbandonedProposals retrieves the proposal details for a list of pre-vote proposals using an array of csfr as params
func (p *Politeia) GetBatchAbandonedProposals() (string, error) {
	tokenInventory, err := p.client.tokenInventory()
	if err != nil {
		return "", err
	}

	var abandonedproposals *Tokens
	if len(tokenInventory.Abandoned) > 20 {
		abandonedproposals = &Tokens{tokenInventory.Abandoned[:20]}
	} else {
		abandonedproposals = &Tokens{tokenInventory.Abandoned}
	}

	proposals, err := p.client.batchProposals(abandonedproposals)
	if err != nil {
		return "", err
	}

	votesStatus, err := p.client.batchVoteSummary(abandonedproposals)
	if err != nil {
		return "", err
	}

	for i := range proposals {
		for j := range votesStatus {
			voteStatus := votesStatus[j]
			proposals[i].VoteStatus = voteStatus
		}
	}

	return p.result("GetBatchAbandonedProposals", proposals)
}

// GetProposalDetails fetches the details of a single proposal
// if the version argument is an empty string, the latest version is used
func (p *Politeia) GetProposalDetails(censorshipToken, version string) (string, error) {
	res, err := p.client.proposalDetails(censorshipToken, version)
	if err != nil {
		return "", err
	}

	return p.result("GetProposalDetails", res)
}

// GetVoteStatus fetches the vote status of a single public proposal
func (p *Politeia) GetVoteStatus(censorshipToken string) (string, error) {
	res, err := p.client.voteStatus(censorshipToken)
	if err != nil {
		return "", err
	}

	return p.result("GetVoteStatus", res)
}
