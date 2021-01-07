package controllers

import (
	"encoding/json"
	"net/http"
	"net/url"

	"k8s.io/klog/v2"
)
	
// DockerValidate is POST by us for validate the process.
// The following parameters are recognized in callback data:
// state (required): Accepted values are success, failure, and error. If the state isn’t success,
// the Webhook chain is interrupted. description: A string containing miscellaneous information 
// that is available on Docker Hub. Maximum 255 characters. context: A string containing the 
// context of the operation. Can be retrieved from the Docker Hub. Maximum 100 characters.
// target_url: The URL where the results of the operation can be found. Can be retrieved on the 
// Docker Hub.
type DockerValidate struct {
	State       string `json:"state"`
	Description string `json:"description"`
	Context     string `json:"context"`
	TargetURL   string `json:"target_url"`
}

// Validate send a POST request to callback_url containing a valid JSON body.
func (v *DockerValidate) Validate(c string) (res map[string]interface{}, err error) {
	var u *DockerRequestPayload

	data := url.Values {
		v.State: {u.Repository.Status},
		v.Description: {u.Repository.FullDescription},
		v.Context: {"test"},
		v.TargetURL: {u.Repository.RepoURL},
	}

	resp, err := http.PostForm(c, data)

	if err != nil {
		klog.Error("error send a POST request", err)
	}

	err = json.NewDecoder(resp.Body).Decode(&res)
	if err != nil{
		klog.Error("error DecodeJSON Body", err)
	}

	return res, err
}