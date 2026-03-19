# Functional requirements for the CLI app
## Overview
* The app uses delegated authentication for user account
* The chosen AuthenticationProvider is DeviceCodeCredential
* The authentication token must support persistent caching so that the user does not have to authenticate for each use

## Commands
### login
* This command will ask the user to log in to their Microsoft 365 account
