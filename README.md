<h1 align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://github.com/raito-io/raito-io.github.io/raw/master/assets/images/logo-vertical-dark%402x.png">
    <img height="250px" src="https://github.com/raito-io/raito-io.github.io/raw/master/assets/images/logo-vertical%402x.png">
  </picture>
</h1>

<h4 align="center">
  Okta plugin for the Raito CLI
</h4>

<p align="center">
    <a href="/LICENSE.md" target="_blank"><img src="https://img.shields.io/badge/license-Apache%202-brightgreen.svg" alt="Software License" /></a>
    <a href="https://github.com/raito-io/cli/actions/workflows/build.yml" target="_blank"><img src="https://img.shields.io/github/workflow/status/raito-io/cli-plugin-okta/Raito%20CLI%20-%20Okta%20Plugin%20-%20Build/main" alt="Build status" /></a>
    <a href="https://codecov.io/gh/raito-io/cli-plugin-okta" target="_blank"><img src="https://img.shields.io/codecov/c/github/raito-io/cli-plugin-okta" alt="Code Coverage" /></a>
</p>

<hr/>

# Raito CLI Plugin - Okta

This Raito CLI plugin will synchronize the users and groups from an Okta account to a specified Raito Identity Store.

## Prerequisites
To use this plugin, you will need

1. The Raito CLI to be correctly installed. You can check out our [documentation](http://docs.raito.io/docs/cli/installation) for help on this.
2. A Raito Cloud account to synchronize your Okta account with. If you don't have this yet, visit our webpage at (https://raito.io) and request a trial account.

## Usage
To use the plugin, add the following snippet to your Raito CLI configuration file (`raito.yml`, by default) under the `targets` section:

```json
  - name: okta1
    connector-name: raito-io/cli-plugin-okta
    identity-store-id: <<Okta IdentityStore ID>>

    okta-domain: <<Your Okta Domain>>
    okta-token: "{{RAITO_OKTA_TOKEN}}"
```

Next, replace the values of the indicated fields with your specific values:
 - `<<Okta IdentityStore ID>>`: the ID of the IdentityStore you created in the Raito Cloud UI.
 - `<<Your Okta Domain>>`: your full okta domain. e.g. `dev-123456789.okta.com`

Optionally, you can set the `okta-excluded-statuses` parameter, where you can specify a comma-separated list of user statuses from Okta. When a user has one of these statuses, this user will not be synced.  
By default, statuses `DEPROVISIONED` and `SUSPENDED` are ignored.  
If you would also like to ignore the `PROVISIONED` status, for example, you can add this to the end of the configuration snippet:
```json
  okta-exclude-statuses: "DEPROVISIONED,SUSPENDED,PROVISIONED"
```

Make sure you have a system variable called `RAITO_OKTA_TOKEN` with a valid Okta token as its value.
For more information on how to create and configure an Okta token, see the [Okta documentation](https://developer.okta.com/docs/guides/create-an-api-token/main/).

You will also need to configure the Raito CLI further to connect to your Raito Cloud account, if that's not set up yet.
A full guide on how to configure the Raito CLI can be found on (http://docs.raito.io/docs/cli/configuration).

## Trying it out

As a first step, you can check if the CLI finds this plugin correctly. In a command-line terminal, execute the following command:
```bash
$> raito info raito-io/cli-plugin-okta
```

This will download the latest version of the plugin (if you don't have it yet) and output the name and version of the plugin, together with all the plugin-specific parameters to configure it.

When you are ready to try out the synchronization for the first time, execute:
```bash
$> raito run
```
This will take the configuration from the `raito.yml` file (in the current working directory) and start a single synchronization.

Note: if you have multiple targets configured in your configuration file, you can run only this target by adding `--only-targets okta1` at the end of the command.
