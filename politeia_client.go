package dcrlibwallet

import (
	"bytes"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	cms "github.com/decred/politeia/politeiawww/api/cms/v1"
	www "github.com/decred/politeia/politeiawww/api/www/v1"
	"github.com/decred/politeia/util"
	"golang.org/x/crypto/sha3"
	"golang.org/x/net/publicsuffix"
)

type client struct {
	httpClient *http.Client
	config     *PoliteiaConfig
}

const (
	host    = "https://proposals.decred.org"
	apiPath = "/api/v1"

	versionPath          = "/version"
	policyPath           = "/policy"
	vettedProposalsPath  = "/proposals/vetted"
	voteStatusPath       = "/proposals/%s/votestatus"
	votesStatusPath      = "/proposals/votestatus"
	proposalDetailsPath  = "/proposals/%s"
	tokenInventoryPath   = "/proposals/tokeninventory"
	batchProposalsPath   = "/proposals/batch"
	batchVoteSummaryPath = "/proposals/batchvotesummary"
	loginPath            = "/login"
)

func newPoliteiaClient(cfg *PoliteiaConfig) (*client, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	tr := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	// Set cookies
	jar, err := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	})
	if err != nil {
		return nil, fmt.Errorf("error initializing cookiejar: %s", err.Error())
	}

	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("error parsing politeia host url: %s", err.Error())
	}
	jar.SetCookies(u, cfg.getCookies())

	httpClient := &http.Client{
		Transport: tr,
		Jar:       jar,
		Timeout:   time.Second * 10,
	}

	c := &client{
		config:     cfg,
		httpClient: httpClient,
	}

	return c, nil
}

func userErrorStatus(e www.ErrorStatusT) string {
	s, ok := www.ErrorStatus[e]
	if ok {
		return s
	}
	s, ok = cms.ErrorStatus[e]
	if ok {
		return s
	}
	return ""
}

func (c *client) makeRequest(method, path string, body interface{}, dest interface{}) error {
	// Setup request
	var requestBody []byte
	var queryParams string
	var err error

	if body != nil {
		switch method {
		case http.MethodGet:
			if body == nil {
				break
			}
			queryParams = "?" + body.(string)
		case http.MethodPost:
			requestBody, err = json.Marshal(body)
			if err != nil {
				return fmt.Errorf("error marshaling request body: %s", err.Error())
			}
		default:
			return fmt.Errorf("unsupported HTTP method: %s", method)
		}
	}

	route := host + apiPath + path + queryParams
	// Create http request
	req, err := http.NewRequest(method, route, bytes.NewReader(requestBody))
	if err != nil {
		return fmt.Errorf("error creating http request: %s", err.Error())
	}
	req.Header.Add("X-CSRF-TOKEN", c.config.CsrfToken)

	// Send request
	r, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		r.Body.Close()
	}()

	responseBody := util.ConvertBodyToByteArray(r.Body, false)

	if r.StatusCode != http.StatusOK {
		switch r.StatusCode {
		case http.StatusNotFound:
			return errors.New("resource not found")
		case http.StatusInternalServerError:
			return errors.New("internal server error")
		case http.StatusForbidden:
			return errors.New(string(responseBody))
		case http.StatusUnauthorized:
			var errResp Err
			if err := json.Unmarshal(responseBody, &errResp); err != nil {
				return err
			}
			return fmt.Errorf("unauthorized: %s", ErrorStatus[errResp.Code])
		case http.StatusBadRequest:
			var errResp Err
			if err := json.Unmarshal(responseBody, &errResp); err != nil {
				return err
			}
			return fmt.Errorf("bad request: %s", ErrorStatus[errResp.Code])
		}
	}

	err = json.Unmarshal(responseBody, dest)
	if err != nil {
		return fmt.Errorf("error unmarshaling response: %s", err.Error())
	}

	if path == loginPath {
		if csrfToken := r.Header.Get(www.CsrfToken); csrfToken != "" {
			err = c.config.saveCSRFToken(csrfToken)
			if err != nil {
				return err
			}
		}

		user := dest.(*User)
		err = c.config.saveSession(c.httpClient.Jar.Cookies(req.URL), user)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *client) version() (*ServerVersion, error) {
	req, err := http.NewRequest(http.MethodGet, versionPath, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating version request: %s", err.Error())
	}
	req.Header.Add(www.CsrfToken, c.config.CsrfToken)

	// Send request
	r, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error fetching politeia server version: %s", err.Error())
	}
	defer func() {
		r.Body.Close()
	}()

	responseBody := util.ConvertBodyToByteArray(r.Body, false)
	if r.StatusCode != http.StatusOK {
		switch r.StatusCode {
		case http.StatusNotFound:
			return nil, errors.New("resource not found")
		case http.StatusInternalServerError:
			return nil, errors.New("internal server error")
		case http.StatusForbidden:
			return nil, errors.New(string(responseBody))
		case http.StatusUnauthorized:
			var errResp Err
			if err := json.Unmarshal(responseBody, &errResp); err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("unauthorized: %s", ErrorStatus[errResp.Code])
		case http.StatusBadRequest:
			var errResp Err
			if err := json.Unmarshal(responseBody, &errResp); err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("bad request: %s", ErrorStatus[errResp.Code])
		}
	}

	var versionResponse ServerVersion
	err = json.Unmarshal(responseBody, &versionResponse)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling version response: %s", err.Error())
	}

	err = c.config.saveCSRFToken(r.Header.Get(www.CsrfToken))
	if err != nil {
		return nil, err
	}

	err = c.config.saveCookies(c.httpClient.Jar.Cookies(req.URL))
	if err != nil {
		return nil, err
	}

	return &versionResponse, nil
}

func (c *client) serverPolicy() (ServerPolicy, error) {
	var serverPolicyResponse ServerPolicy
	err := c.makeRequest(http.MethodGet, policyPath, nil, &serverPolicyResponse)

	return serverPolicyResponse, err
}

func (c *client) batchProposals(censorshipTokens *Tokens) ([]Proposal, error) {
	b, err := json.Marshal(censorshipTokens)
	if err != nil {
		return nil, err
	}

	var result Proposals
	err = c.makeRequest(http.MethodPost, batchProposalsPath, b, &result)
	if err != nil {
		return nil, err
	}

	return result.Proposals, err
}

func (c *client) proposalDetails(censorshipToken, version string) (Proposal, error) {
	var queryParams []byte
	if version != "" {
		queryParams = []byte("version=" + version)
	}

	var proposalResult ProposalResult
	err := c.makeRequest(http.MethodGet, fmt.Sprintf(proposalDetailsPath, censorshipToken), queryParams, &proposalResult)
	return proposalResult.Proposal, err
}

func (c *client) voteStatus(censorshipToken string) (VoteStatus, error) {
	var voteStatus VoteStatus

	err := c.makeRequest(http.MethodGet, fmt.Sprintf(voteStatusPath, censorshipToken), nil, &voteStatus)
	return voteStatus, err
}

func (c *client) votesStatus() (VotesStatus, error) {
	var votesStatus VotesStatus

	err := c.makeRequest(http.MethodGet, votesStatusPath, nil, &votesStatus)
	return votesStatus, err
}

func (c *client) tokenInventory() (*TokenInventory, error) {
	var tokenInventory TokenInventory

	err := c.makeRequest(http.MethodGet, tokenInventoryPath, nil, &tokenInventory)
	if err != nil {
		return nil, err
	}

	return &tokenInventory, nil
}

func (c *client) batchVoteSummary(censorshipTokens *Tokens) ([]VoteStatus, error) {
	if censorshipTokens == nil {
		return nil, errors.New("censorship token cannot be empty")
	}

	b, err := json.Marshal(censorshipTokens)
	if err != nil {
		return nil, err
	}

	var result VotesStatus
	err = c.makeRequest(http.MethodPost, batchVoteSummaryPath, b, &result)
	if err != nil {
		return nil, err
	}

	return result.VotesStatus, err
}

func (c *client) login(email, password string) (*User, error) {
	// fetch csrf token if it doesnt exist
	if c.config.CsrfToken == "" {
		_, err := c.version()
		if err != nil {
			return nil, err
		}
	}

	h := sha3.New256()
	h.Write([]byte(password))

	login := Login{
		Email:    email,
		Password: hex.EncodeToString(h.Sum(nil)),
	}

	var user User
	err := c.makeRequest(http.MethodPost, loginPath, login, &user)
	if err != nil {
		return nil, err
	}

	return &user, nil
}
