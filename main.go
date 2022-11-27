package main

import (
	"fmt"

	"github.com/hashicorp/go-hclog"
	"github.com/raito-io/cli/base"
	"github.com/raito-io/cli/base/info"
	"github.com/raito-io/cli/base/util/plugin"
	"github.com/raito-io/cli/base/wrappers"
)

var version = "0.0.0"

var logger hclog.Logger

func main() {
	logger = base.Logger()
	logger.SetLevel(hclog.Debug)

	err := base.RegisterPlugins(wrappers.IdentityStoreSync(&IdentityStoreSyncer{}), &info.InfoImpl{
		Info: plugin.PluginInfo{
			Name:    "Okta",
			Version: plugin.ParseVersion(version),
			Description: `Okta integration for Raito. 
It only implements the Identity Store syncer interface to fetch users and groups from Okta to import them into Raito.`,
			Parameters: []plugin.ParameterInfo{
				{Name: "okta-domain", Description: "The full okta domain to connect to. For example, mydomain.okta.com", Mandatory: true},
				{Name: "okta-token", Description: "The secret okta token to use to authenticate against the okta domain.", Mandatory: true},
			},
		},
	})

	if err != nil {
		logger.Error(fmt.Sprintf("error while registering plugins: %s", err.Error()))
	}
}
