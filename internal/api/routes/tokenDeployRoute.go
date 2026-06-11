package routes

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ifeanyiecheruo/morsel/internal/tokens"
	"github.com/ifeanyiecheruo/morsel/platform"
)

func HandleTokenDeployRoute(creds platform.CredentialProvider, signingKey []byte) func(http.ResponseWriter, *http.Request) error {
	return func(resp http.ResponseWriter, req *http.Request) error {
		var body struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.Token == "" {
			return &APIError{
				HTTPStatus: http.StatusBadRequest,
				Code:       "invalid_request",
				Message:    "request body must include a non-empty token field",
				Remedy:     "provide a valid deploy identity token obtained from Platform.DeployToken()",
			}
		}

		slug, err := creds.ValidateDeployToken(req.Context(), body.Token)
		if err != nil {
			return &APIError{
				HTTPStatus: http.StatusUnauthorized,
				Code:       "invalid_token",
				Message:    "deploy identity token is invalid or expired",
				Remedy:     "re-run morsel app deploy to obtain a fresh deploy identity token",
			}
		}

		accessToken, err := tokens.IssueToken(signingKey, tokens.CreateDeployClaims(slug))
		if err != nil {
			return fmt.Errorf("issue deploy token: %w", err)
		}

		resp.Header().Set("Content-Type", "application/json")
		return json.NewEncoder(resp).Encode(struct { //nolint:errcheck
			AccessToken string `json:"access_token"`
			ExpiresIn   int    `json:"expires_in"`
		}{
			AccessToken: accessToken,
			ExpiresIn:   600,
		})
	}
}
