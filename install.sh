# This tool is meant for development reasons.
# It builds the plugin and installs it in the plugins folder of the cli repository (considering these repositories are checked out in the same top-level folder)
go build -o ../cli/plugins/okta .
