package main

import (
	"encoding/json"
	"fmt"
	isb "github.com/raito-io/cli/base/identity_store"
	"github.com/raito-io/cli/common/api"
	"github.com/raito-io/cli/common/api/identity_store"
	"github.com/raito-io/cli/common/util/url"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

var statusesToSkip = map[string]struct{}{
	"PROVISIONED":   {},
	"DEPROVISIONED": {},
	"SUSPENDED":     {},
}

type IdentityStoreSyncer struct {
	config  *identity_store.IdentityStoreSyncConfig
	baseUrl string
	token   string
}

func (s *IdentityStoreSyncer) SyncIdentityStore(config *identity_store.IdentityStoreSyncConfig) identity_store.IdentityStoreSyncResult {
	s.config = config

	oktaDomain := config.GetString(OktaDomain)
	if oktaDomain == "" {
		return identity_store.IdentityStoreSyncResult{
			Error: api.CreateMissingInputParameterError(OktaDomain),
		}
	}
	s.baseUrl = "https://" + s.cleanOktaDomain(oktaDomain)
	s.token = config.GetString(OktaToken)
	if s.token == "" {
		return identity_store.IdentityStoreSyncResult{
			Error: api.CreateMissingInputParameterError(OktaToken),
		}
	}

	fileCreator, err := isb.NewIdentityStoreFileCreator(config)
	if err != nil {
		return identity_store.IdentityStoreSyncResult{
			Error: api.ToErrorResult(err),
		}
	}
	defer fileCreator.Close()

	start := time.Now()

	userGroups := make(map[string][]string)
	err = s.syncGroups(&fileCreator, userGroups)
	if err != nil {
		return identity_store.IdentityStoreSyncResult{
			Error: api.ToErrorResult(err),
		}
	}
	err = s.syncUsers(&fileCreator, userGroups)
	if err != nil {
		return identity_store.IdentityStoreSyncResult{
			Error: api.ToErrorResult(err),
		}
	}

	sec := time.Since(start).Round(time.Millisecond)

	logger.Info(fmt.Sprintf("Done fetching %d users and %d groups from Okta in %s", fileCreator.GetUserCount(), fileCreator.GetGroupCount(), sec))

	return identity_store.IdentityStoreSyncResult{}
}

func (s *IdentityStoreSyncer) cleanOktaDomain(input string) string {
	domain := url.CutOffSchema(input)
	domain = url.CutOffSuffix(domain, "/")
	return domain
}

func (s *IdentityStoreSyncer) syncUsers(isFileCreator *isb.IdentityStoreFileCreator, userGroups map[string][]string) error {
	start := time.Now()

	url := s.baseUrl + "/api/v1/users?limit=200"
	for url != "" {
		var err error
		url, err = s.readUsersFromURL(url, isFileCreator, userGroups)
		if err != nil {
			return err
		}
	}

	sec := time.Since(start).Round(time.Millisecond)

	logger.Info(fmt.Sprintf("Fetched %d users from Okta in %s", (*isFileCreator).GetUserCount(), sec))
	return nil
}

func (s *IdentityStoreSyncer) syncGroups(isFileCreator *isb.IdentityStoreFileCreator, userGroups map[string][]string) error {
	start := time.Now()

	url := s.baseUrl + "/api/v1/groups?limit=200"
	for url != "" {
		var err error
		url, err = s.readGroupsFromURL(url, isFileCreator, userGroups)
		if err != nil {
			return err
		}
	}

	sec := time.Since(start).Round(time.Millisecond)

	logger.Info(fmt.Sprintf("Fetched %d groups from Okta in %s", (*isFileCreator).GetGroupCount(), sec))
	return nil
}

func (s *IdentityStoreSyncer) readGroupsFromURL(url string, isFileCreator *isb.IdentityStoreFileCreator, userGroups map[string][]string) (string, error) {
	resp, err := s.doRequest(url)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("Received HTTP error code %d when calling %q: %s", resp.StatusCode, url, resp.Status)
	}
	groupEntities := make([]groupEntity, 0, 200)
	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return "", fmt.Errorf("Error while reading body from HTTP GET request to %q: %s", url, err.Error())
	}
	err = json.Unmarshal(body, &groupEntities)
	if err != nil {
		return "", fmt.Errorf("Error while parsing body from HTTP GET request to %q: %s", url, err.Error())
	}

	groups := make([]isb.Group, 0, 200)
	for _, groupEntity := range groupEntities {
		logger.Debug(fmt.Sprintf("Handling group %q.", groupEntity.Profile.Name))
		group := isb.Group{
			ExternalId:  groupEntity.Id,
			DisplayName: groupEntity.Profile.Name,
			Description: groupEntity.Profile.Description,
			Name:        groupEntity.Profile.Name,
		}
		groups = append(groups, group)
		err = s.fetchUsersForGroup(groupEntity.Links.Users.Href, groupEntity.Id, userGroups)
		if err != nil {
			return "", fmt.Errorf("Error while fetching Users for group %s: %s", groupEntity.Id, err.Error())
		}
	}
	err = (*isFileCreator).AddGroups(groups)
	if err != nil {
		return "", fmt.Errorf("Error while adding groups to the importer: %s", err.Error())
	}

	return s.getNextLink(resp), nil
}

func (s *IdentityStoreSyncer) fetchUsersForGroup(url string, group string, userGroups map[string][]string) error {
	resp, err := s.doRequest(url)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("Received HTTP error code %d when calling %q: %s", resp.StatusCode, url, resp.Status)
	}
	userEntities := make([]userIdEntity, 0, 200)
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Error while reading body from HTTP GET request to %q: %s", url, err.Error())
	}
	err = json.Unmarshal(body, &userEntities)
	if err != nil {
		return fmt.Errorf("Error while parsin body from HTTP GET request to %q: %s", url, err.Error())
	}

	for _, userEntity := range userEntities {
		userGroups[userEntity.Id] = append(userGroups[userEntity.Id], group)
	}

	next := s.getNextLink(resp)
	if next != "" {
		return s.fetchUsersForGroup(next, group, userGroups)
	}
	return nil
}

func (s *IdentityStoreSyncer) readUsersFromURL(url string, isFileCreator *isb.IdentityStoreFileCreator, userGroups map[string][]string) (string, error) {
	resp, err := s.doRequest(url)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("Received HTTP error code %d when calling %q: %s", resp.StatusCode, url, resp.Status)
	}
	userEntities := make([]userEntity, 0, 200)
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("Error while reading body from HTTP GET request to %q: %s", url, err.Error())
	}
	err = json.Unmarshal(body, &userEntities)
	if err != nil {
		return "", fmt.Errorf("Error while parsin body from HTTP GET request to %q: %s", url, err.Error())
	}

	users := make([]isb.User, 0, 200)
	for _, userEntity := range userEntities {
		if userEntity.Profile.Login != "" {
			logger.Debug(fmt.Sprintf("Handling user %q.", userEntity.Profile.Login))

			if _, ok := statusesToSkip[strings.ToUpper(userEntity.Status)]; ok {
				logger.Debug(fmt.Sprintf("Skipping user %q with status %q", userEntity.Profile.Login, userEntity.Status))
				continue
			}

			tags := make(map[string]interface{})
			if userEntity.Profile.Department != "" {
				tags["Department"] = userEntity.Profile.Department
			}
			if userEntity.Profile.Division != "" {
				tags["Division"] = userEntity.Profile.Division
			}
			if userEntity.Profile.Organization != "" {
				tags["Organization"] = userEntity.Profile.Organization
			}
			if userEntity.Profile.CostCenter != "" {
				tags["CostCenter"] = userEntity.Profile.CostCenter
			}
			if userEntity.Profile.CountryCode != "" {
				tags["CountryCode"] = userEntity.Profile.CountryCode
			}
			if userEntity.Profile.State != "" {
				tags["State"] = userEntity.Profile.State
			}
			if userEntity.Profile.City != "" {
				tags["City"] = userEntity.Profile.City
			}
			if userEntity.Profile.Title != "" {
				tags["Title"] = userEntity.Profile.Title
			}

			user := isb.User{
				ExternalId:       userEntity.Id,
				UserName:         userEntity.Profile.Login,
				Name:             userEntity.Profile.FirstName + " " + userEntity.Profile.LastName,
				Email:            userEntity.Profile.Email,
				GroupExternalIds: userGroups[userEntity.Id],
				Tags:             tags,
			}
			users = append(users, user)
		}

	}
	err = (*isFileCreator).AddUsers(users)
	if err != nil {
		return "", fmt.Errorf("Error while adding users to the importer: %s", err.Error())
	}

	return s.getNextLink(resp), nil
}

func (s *IdentityStoreSyncer) doRequest(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("Error while creating HTTP GET request to %q: %s", url, err.Error())
	}
	req.Header.Set("Authorization", "SSWS "+s.token)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, api.CreateSourceConnectionError(url, err.Error())
	}
	return resp, nil
}

func (s *IdentityStoreSyncer) getNextLink(resp *http.Response) string {
	links := resp.Header.Values("link")
	for _, link := range links {
		if strings.HasSuffix(link, "rel=\"next\"") {
			return link[strings.Index(link, "<")+1 : strings.Index(link, ">")]
		}
	}
	return ""
}

type userEntity struct {
	Id      string
	Status  string
	Profile struct {
		Login        string `json:"login"`
		FirstName    string `json:"firstName"`
		LastName     string `json:"lastName"`
		Email        string `json:"email"`
		Department   string `json:"department"`
		Division     string `json:"division"`
		Organization string `json:"organization"`
		CostCenter   string `json:"costCenter"`
		CountryCode  string `json:"countryCode"`
		State        string `json:"state"`
		City         string `json:"city"`
		Title        string `json:"title"`
	}
}

// Type to only retrieve the ID of the user
type userIdEntity struct {
	Id string
}

type groupEntity struct {
	Id      string
	Profile struct {
		Name        string
		Description string
	}
	Links struct {
		Users struct {
			Href string
		}
	} `json:"_links"`
}
