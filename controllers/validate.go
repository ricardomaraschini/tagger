// Package controllers is the layer for imput date in the application.
package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// DockerRequestPayloadNew is sent by docker hub whenever a new push happen to a
// repository.
type DockerRequestPayloadNew struct {
	CallbackURL string `json:"callback_url"`
	PushData    struct {
		Images   []string `json:"images"`
		PushedAt int      `json:"pushed_at"`
		Pusher   string   `json:"pusher"`
		Tag      string   `json:"tag"`
	} `json:"push_data"`
	Repository struct {
		CommentCount    int    `json:"comment_count"`
		DateCreated     int    `json:"date_created"`
		Description     string `json:"description"`
		Dockerfile      string `json:"dockerfile"`
		FullDescription string `json:"full_description"`
		IsOfficial      bool   `json:"is_official"`
		IsPrivate       bool   `json:"is_private"`
		IsTrusted       bool   `json:"is_trusted"`
		Name            string `json:"name"`
		Namespace       string `json:"namespace"`
		Owner           string `json:"owner"`
		RepoName        string `json:"repo_name"`
		RepoURL         string `json:"repo_url"`
		StarCount       int    `json:"star_count"`
		Status          string `json:"status"`
	} `json:"repository"`
}
	
// DockerValidate is POST by us for validate the process.
// The following parameters are recognized in callback data:
// state (required): Accepted values are success, failure, and error. If the state isn’t success, the Webhook chain is interrupted.
// description: A string containing miscellaneous information that is available on Docker Hub. Maximum 255 characters.
// context: A string containing the context of the operation. Can be retrieved from the Docker Hub. Maximum 100 characters.
// target_url: The URL where the results of the operation can be found. Can be retrieved on the Docker Hub.
type DockerValidate struct {
	State       string `json:"state"`
	Description string `json:"description"`
	Context     string `json:"context"`
	TargetURL   string `json:"target_url"`
}

// requestJSONPayload retrieve the callback_url value in the request’s JSON payload.
func (r *DockerRequestPayloadNew) requestJSONPayload() (c string, err error) {
	c = r.CallbackURL
	if err != nil {
		fmt.Println("error to retrieve callback_url with requestJSONPayload", err)
	}
	
	return c, err
}

// Validate send a POST request to callback_url containing a valid JSON body.
func (v *DockerValidate) Validate(c string) (res map[string]interface{}, err error) {
	var u *DockerRequestPayloadNew

	data := url.Values {
		v.State: {u.Repository.Status},
		v.Description: {u.Repository.FullDescription},
		v.Context: {"test"},
		v.TargetURL: {u.Repository.RepoURL},
	}

	resp, err := http.PostForm(c, data)

	if err != nil {
		fmt.Println("error send a POST request", err)
	}

	json.NewDecoder(resp.Body).Decode(&res)

	return res, err
}