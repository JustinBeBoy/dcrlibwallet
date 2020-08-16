package dcrlibwallet

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	www "github.com/decred/politeia/politeiawww/api/www/v1"
	"github.com/decred/politeia/util"
)

type client struct {
	httpClient *http.Client

	policy             *ServerPolicy
	csrfToken          string
	cookies            []*http.Cookie
	csrfTokenExpiresAt time.Time
}

const (
	host    = "https://proposals.decred.org"
	apiPath = "/api/v1"

	versionPath          = "/version"
	policyPath           = "/policy"
	votesStatusPath      = "/proposals/votestatus"
	tokenInventoryPath   = "/proposals/tokeninventory"
	batchProposalsPath   = "/proposals/batch"
	batchVoteSummaryPath = "/proposals/batchvotesummary"

	ErrNotFound uint16 = iota + 1
	ErrUnknownError
)

var (
	ErrorStatus = map[uint16]string{
		ErrNotFound:     "no record found",
		ErrUnknownError: "an unknown error occurred",
	}
)

func newPoliteiaClient() *client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	httpClient := &http.Client{
		Transport: tr,
		Timeout:   time.Second * 30,
	}

	return &client{
		httpClient: httpClient,
	}
}

func (c *client) getRequestBody(method string, body interface{}) ([]byte, error) {
	if body == nil {
		return nil, nil
	}

	if method == http.MethodPost {
		if requestBody, ok := body.([]byte); ok {
			return requestBody, nil
		}
	} else if method == http.MethodGet {
		if requestBody, ok := body.(map[string]string); ok {
			params := url.Values{}
			for key, val := range requestBody {
				params.Add(key, val)
			}
			return []byte(params.Encode()), nil
		}
	}

	return nil, errors.New("invalid request body")
}

func (c *client) makeRequest(method, path string, body interface{}, dest interface{}) error {
	var err error
	var requestBody []byte

	if c.csrfToken == "" {
		_, err := c.version()
		if err != nil {
			return err
		}
	}

	route := host + apiPath + path
	if body != nil {
		requestBody, err = c.getRequestBody(method, body)
		if err != nil {
			return err
		}
	}

	if method == http.MethodGet && requestBody != nil {
		route += string(requestBody)
	}

	// Create http request
	req, err := http.NewRequest(method, route, nil)
	if err != nil {
		return fmt.Errorf("error creating http request: %s", err.Error())
	}
	if method == http.MethodPost && requestBody != nil {
		req.Body = ioutil.NopCloser(bytes.NewReader(requestBody))
	}
	req.Header.Add(www.CsrfToken, c.csrfToken)

	for _, cookie := range c.cookies {
		req.AddCookie(cookie)
	}

	// Send request
	r, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		r.Body.Close()
	}()

	responseBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}

	if r.StatusCode != http.StatusOK {
		return c.handleError(r.StatusCode, responseBody)
	}

	err = json.Unmarshal(responseBody, dest)
	if err != nil {
		return fmt.Errorf("error unmarshaling response: %s", err.Error())
	}

	return nil
}

func (c *client) handleError(statusCode int, responseBody []byte) error {
	switch statusCode {
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

	return errors.New("unknown error")
}

func (c *client) version() (*ServerVersion, error) {
	route := host + apiPath + versionPath
	req, err := http.NewRequest(http.MethodGet, route, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating version request: %s", err.Error())
	}
	req.Header.Add(www.CsrfToken, c.csrfToken)

	// Send request
	r, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error fetching politeia server version: %s", err.Error())
	}
	defer func() {
		r.Body.Close()
	}()

	c.cookies = r.Cookies()

	responseBody := util.ConvertBodyToByteArray(r.Body, false)
	if r.StatusCode != http.StatusOK {
		return nil, c.handleError(r.StatusCode, responseBody)
	}

	var versionResponse ServerVersion
	err = json.Unmarshal(responseBody, &versionResponse)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling version response: %s", err.Error())
	}

	newCsrfToken := r.Header.Get(www.CsrfToken)
	if newCsrfToken != "" {
		c.csrfToken = newCsrfToken
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

func (c *client) votesStatus() ([]VoteStatus, error) {
	var votesStatus VotesStatus
	err := c.makeRequest(http.MethodGet, votesStatusPath, nil, &votesStatus)
	if err != nil {
		return nil, err
	}

	return votesStatus.VotesStatus, nil
}

func (c *client) tokenInventory() (*TokenInventory, error) {
	var tokenInventory TokenInventory

	err := c.makeRequest(http.MethodGet, tokenInventoryPath, nil, &tokenInventory)
	if err != nil {
		return nil, err
	}

	return &tokenInventory, nil
}

func (c *client) batchVoteSummary(censorshipTokens *Tokens) (*VoteSummaries, error) {
	if censorshipTokens == nil {
		return nil, errors.New("censorship token cannot be empty")
	}

	b, err := json.Marshal(censorshipTokens)
	if err != nil {
		return nil, err
	}

	var result VoteSummaries
	err = c.makeRequest(http.MethodPost, batchVoteSummaryPath, b, &result)
	if err != nil {
		return nil, err
	}

	return &result, err
}
