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
	db     *storm.DB

	syncQuitChan          chan struct{}
	notificationListeners PoliteiaNotificationListeners
}

const (
	proposalsDbName = "proposals.db"
)

func NewPoliteia(rootDir string) (*Politeia, error) {
	db, err := storm.Open(filepath.Join(rootDir, proposalsDbName))
	if err != nil {
		return nil, fmt.Errorf("error opening proposals database: %s", err.Error())
	}

	err = db.Init(&Proposal{})
	if err != nil {
		return nil, fmt.Errorf("error initializing proposals database: %s", err.Error())
	}

	return &Politeia{
		client: newPoliteiaClient(),
		db:     db,
	}, nil
}

func (p *Politeia) Shutdown() {
	close(p.syncQuitChan)
	p.db.Close()
}

// GetProposalsRaw fetches and returns a proposals from the db
func (p *Politeia) GetProposalsRaw(category ProposalCategory, offset, limit int32, newestFirst bool) ([]Proposal, error) {
	query := p.prepareQuery(category, offset, limit, newestFirst)

	var proposals []Proposal
	err := query.Find(&proposals)
	if err != nil && err != storm.ErrNotFound {
		return nil, fmt.Errorf("error fetching proposals: %s", err.Error())
	}

	return proposals, nil
}

// GetProposals returns the result of GetProposalsRaw as a JSON string
func (p *Politeia) GetProposals(category ProposalCategory, offset, limit int32, newestFirst bool) (string, error) {
	return processResult(p.GetProposalsRaw(category, offset, limit, newestFirst))
}

// GetProposalRaw fetches and returns a single proposal specified by it's censorship record token
func (p *Politeia) GetProposalRaw(censorshipToken string) (*Proposal, error) {
	var proposal Proposal
	err := p.db.Select(q.Eq("Token", censorshipToken)).First(&proposal)
	if err != nil {
		return nil, err
	}

	return &proposal, nil
}

// GetProposal returns the result of GetProposalRaw as a JSON string
func (p *Politeia) GetProposal(censorshipToken string) (string, error) {
	return processResult(p.GetProposalRaw(censorshipToken))
}

// GetProposalByIDRaw fetches and returns a single proposal specified by it's ID
func (p *Politeia) GetProposalByIDRaw(proposalID int) (*Proposal, error) {
	var proposal Proposal
	err := p.db.Select(q.Eq("ID", proposalID)).First(&proposal)
	if err != nil {
		return nil, err
	}

	return &proposal, nil
}

// GetProposalByID returns the result of GetProposalByIDRaw as a JSON string
func (p *Politeia) GetProposalByID(proposalID int) (string, error) {
	return processResult(p.GetProposalByIDRaw(proposalID))
}

// GetVoteStatusRaw fetches and returns the vote status of a proposal
func (p *Politeia) GetVoteStatusRaw(censorshipToken string) (*VoteStatus, error) {
	proposal, err := p.GetProposalRaw(censorshipToken)
	if err != nil {
		return nil, err
	}

	return &proposal.VoteStatus, nil
}

// GetVoteStatus returns the result of GetVoteStatusRaw as a JSON string
func (p *Politeia) GetVoteStatus(censorshipToken string) (string, error) {
	return processResult(p.GetVoteStatusRaw(censorshipToken))
}

func (p *Politeia) count(category ProposalCategory) (int, error) {
	var proposals []Proposal

	err := p.db.Select(q.Eq("Category", category)).Find(&proposals)
	if err != nil {
		return 0, fmt.Errorf("error fetching number of pre proposals: %s", err.Error())
	}

	return len(proposals), nil
}

// CountPreRaw gets the total count of proposals in discussion
func (p *Politeia) CountPreRaw() (int, error) {
	return p.count(PreProposals)
}

// CountPre returns the result of CountPreRaw as a JSON string
func (p *Politeia) CountPre() (string, error) {
	return processResult(p.CountPreRaw())
}

// CountActiveRaw gets the total count of active proposals
func (p *Politeia) CountActiveRaw() (int, error) {
	return p.count(ActiveProposals)
}

// CountActive returns the result of CountActiveRaw as a JSON string
func (p *Politeia) CountActive() (string, error) {
	return processResult(p.CountActive())
}

// CountApprovedRaw gets the total count of approved proposals
func (p *Politeia) CountApprovedRaw() (int, error) {
	return p.count(ApprovedProposals)
}

// CountApproved returns the result of CountApprovedRaw as a JSON string
func (p *Politeia) CountApproved() (string, error) {
	return processResult(p.CountApprovedRaw())
}

// CountRejectedRaw gets the total count of rejected proposals
func (p *Politeia) CountRejectedRaw() (int, error) {
	return p.count(RejectedProposals)
}

// CountRejected returns the result of CountRejected as a JSON string
func (p *Politeia) CountRejected() (string, error) {
	return processResult(p.CountRejectedRaw())
}

// CountAbandonedRaw gets the total count of abandoned proposals
func (p *Politeia) CountAbandonedRaw() (int, error) {
	return p.count(AbandonedProposals)
}

// CountAbandoned returns the result of CountAbandonedRaw as a JSON string
func (p *Politeia) CountAbandoned() (string, error) {
	return processResult(p.CountAbandonedRaw())
}

func (p *Politeia) prepareQuery(category ProposalCategory, offset, limit int32, newestFirst bool) (query storm.Query) {
	switch category {
	case AllProposals:
		query = p.db.Select(
			q.True(),
		)
	default:
		query = p.db.Select(
			q.Eq("Category", category),
		)
	}

	if offset > 0 {
		query = query.Skip(int(offset))
	}

	if limit > 0 {
		query = query.Limit(int(limit))
	}

	if newestFirst {
		query = query.OrderBy("Timestamp").Reverse()
	} else {
		query = query.OrderBy("Timestamp")
	}

	return
}

func processResult(result interface{}, err error) (string, error) {
	var response Response

	if err != nil {
		response.Error = &ResponseError{}
		if err == storm.ErrNotFound {
			response.Error.Code = ErrNotFound
			response.Error.Message = ErrorStatus[ErrNotFound]
		} else {
			response.Error.Code = ErrUnknownError
			response.Error.Message = ErrorStatus[ErrUnknownError]
		}
	} else {
		response.Result = result
	}

	responseB, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("error marshalling result: %s", err.Error())
	}

	return string(responseB), nil
}
