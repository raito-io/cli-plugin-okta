package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	isb "github.com/raito-io/cli/base/identity_store"
	"github.com/raito-io/cli/base/util/config"
	e "github.com/raito-io/cli/base/util/error"
	"github.com/raito-io/cli/base/util/url"
	"github.com/raito-io/cli/base/wrappers"
)

var defaultStatusesToSkip = map[string]struct{}{
	"DEPROVISIONED": {},
	"SUSPENDED":     {},
}

type IdentityStoreSyncer struct {
	baseUrl        string
	token          string
	statusesToSkip map[string]struct{}
}

func (s *IdentityStoreSyncer) GetIdentityStoreMetaData() isb.MetaData {
	logger.Debug("Returning meta data for Okta identity store")

	return isb.MetaData{
		Type: "okta",
	}
}

func (s *IdentityStoreSyncer) SyncIdentityStore(ctx context.Context, identityHandler wrappers.IdentityStoreIdentityHandler, configMap *config.ConfigMap) error {
	oktaDomain := configMap.GetString(OktaDomain)
	if oktaDomain == "" {
		return e.CreateMissingInputParameterError(OktaDomain)
	}

	s.baseUrl = "https://" + s.cleanOktaDomain(oktaDomain)
	s.token = configMap.GetString(OktaToken)

	if s.token == "" {
		return e.CreateMissingInputParameterError(OktaToken)
	}

	excludes := configMap.GetString(OktaExcludeStatuses)
	if excludes != "" {
		s.statusesToSkip = make(map[string]struct{})
		for _, status := range strings.Split(excludes, ",") {
			s.statusesToSkip[strings.TrimSpace(status)] = struct{}{}
		}
	} else {
		s.statusesToSkip = defaultStatusesToSkip
	}

	userGroups := make(map[string][]string)
	err := s.syncGroups(identityHandler, userGroups)

	if err != nil {
		return err
	}
	err = s.syncUsers(identityHandler, userGroups)

	if err != nil {
		return err
	}

	return nil
}

func (s *IdentityStoreSyncer) cleanOktaDomain(input string) string {
	domain := url.CutOffSchema(input)
	domain = url.CutOffSuffix(domain, "/")

	return domain
}

func (s *IdentityStoreSyncer) syncUsers(identityHandler wrappers.IdentityStoreIdentityHandler, userGroups map[string][]string) error {
	url := s.baseUrl + "/api/v1/users?limit=200"

	for url != "" {
		var err error
		url, err = s.readUsersFromURL(url, identityHandler, userGroups)

		if err != nil {
			return err
		}
	}

	return nil
}

func (s *IdentityStoreSyncer) syncGroups(identityHandler wrappers.IdentityStoreIdentityHandler, userGroups map[string][]string) error {
	url := s.baseUrl + "/api/v1/groups?limit=200"

	for url != "" {
		var err error
		url, err = s.readGroupsFromURL(url, identityHandler, userGroups)

		if err != nil {
			return err
		}
	}

	return nil
}

func (s *IdentityStoreSyncer) readGroupsFromURL(url string, identityHandler wrappers.IdentityStoreIdentityHandler, userGroups map[string][]string) (string, error) {
	resp, err := s.doRequest(url)
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("received HTTP error code %d when calling %q: %s", resp.StatusCode, url, resp.Status)
	}

	groupEntities := make([]groupEntity, 0, 200)
	body, err := io.ReadAll(resp.Body)

	defer resp.Body.Close()

	if err != nil {
		return "", fmt.Errorf("error while reading body from HTTP GET request to %q: %s", url, err.Error())
	}

	err = json.Unmarshal(body, &groupEntities)
	if err != nil {
		return "", fmt.Errorf("error while parsing body from HTTP GET request to %q: %s", url, err.Error())
	}

	for _, groupEntity := range groupEntities {
		logger.Debug(fmt.Sprintf("Handling group %q.", groupEntity.Profile.Name))
		group := isb.Group{
			ExternalId:  groupEntity.Id,
			DisplayName: groupEntity.Profile.Name,
			Description: groupEntity.Profile.Description,
			Name:        groupEntity.Profile.Name,
		}

		err = identityHandler.AddGroups(&group)
		if err != nil {
			return "", err
		}

		err = s.fetchUsersForGroup(groupEntity.Links.Users.Href, groupEntity.Id, userGroups)
		if err != nil {
			return "", fmt.Errorf("error while fetching Users for group %s: %s", groupEntity.Id, err.Error())
		}
	}

	return s.getNextLink(resp), nil
}

func (s *IdentityStoreSyncer) fetchUsersForGroup(url string, group string, userGroups map[string][]string) error {
	resp, err := s.doRequest(url)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 300 {
		return fmt.Errorf("eeceived HTTP error code %d when calling %q: %s", resp.StatusCode, url, resp.Status)
	}

	userEntities := make([]userIdEntity, 0, 200)

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return fmt.Errorf("error while reading body from HTTP GET request to %q: %s", url, err.Error())
	}
	err = json.Unmarshal(body, &userEntities)

	if err != nil {
		return fmt.Errorf("error while parsin body from HTTP GET request to %q: %s", url, err.Error())
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

func (s *IdentityStoreSyncer) readUsersFromURL(url string, identityHandler wrappers.IdentityStoreIdentityHandler, userGroups map[string][]string) (string, error) {
	resp, err := s.doRequest(url)

	if err != nil {
		return "", err
	}

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("received HTTP error code %d when calling %q: %s", resp.StatusCode, url, resp.Status)
	}

	userEntities := make([]*userEntity, 0, 200)

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return "", fmt.Errorf("error while reading body from HTTP GET request to %q: %s", url, err.Error())
	}
	err = json.Unmarshal(body, &userEntities)

	if err != nil {
		return "", fmt.Errorf("error while parsin body from HTTP GET request to %q: %s", url, err.Error())
	}

	for _, userEntity := range userEntities {
		if userEntity.Profile.Login == "" {
			continue
		}

		logger.Debug(fmt.Sprintf("Handling user %q.", userEntity.Profile.Login))

		if _, ok := s.statusesToSkip[strings.ToUpper(userEntity.Status)]; ok {
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

		err = identityHandler.AddUsers(&user)
		if err != nil {
			return "", err
		}
	}

	return s.getNextLink(resp), nil
}

func (s *IdentityStoreSyncer) doRequest(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("error while creating HTTP GET request to %q: %s", url, err.Error())
	}

	req.Header.Set("Authorization", "SSWS "+s.token)
	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		return nil, e.CreateSourceConnectionError(url, err.Error())
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
