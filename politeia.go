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

	syncQuitChan chan struct{}
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

	client, err := newPoliteiaClient()
	if err != nil {
		return nil, err
	}

	return &Politeia{
		client: client,
		db:     db,
	}, nil
}

func (p *Politeia) Shutdown() {
	p.db.Close()
}

func (p *Politeia) GetProposalsRaw(category ProposalCategory, offset, limit int32, newestFirst bool) ([]Proposal, error) {
	query := p.prepareQuery(category, offset, limit, newestFirst)

	var proposals []Proposal
	err := query.Find(&proposals)
	if err != nil && err != storm.ErrNotFound {
		return nil, fmt.Errorf("error fetching proposals: %s", err.Error())
	}

	return proposals, nil
}

func (p *Politeia) GetProposals(category ProposalCategory, offset, limit int32, newestFirst bool) (string, error) {
	return processResult(p.GetProposalsRaw(category, offset, limit, newestFirst))
}

func (p *Politeia) GetProposalByIDRaw(proposalID int) (*Proposal, error) {
	var proposal Proposal
	err := p.db.Select(q.Eq("ID", proposalID)).First(&proposal)
	if err != nil {
		return nil, err
	}

	return &proposal, nil
}

func (p *Politeia) GetProposalByID(proposalID int) (string, error) {
	return processResult(p.GetProposalByIDRaw(proposalID))
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
