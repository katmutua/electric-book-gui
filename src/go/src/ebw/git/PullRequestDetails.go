package git

import (
	"github.com/google/go-github/github"
	"net/http"
	"io/ioutil"
)

type PullRequestDetails struct {
	ChangedFiles []string
	Content      string
}

func PullRequestChanges(client *Client, pr *github.PullRequest, repoName string) (PullRequestDetails, error) {
	var cf [] string
	prc := PullRequestDetails{}

	c, _, err := client.PullRequests.ListFiles(client.Context, client.Username,
		repoName, pr.GetNumber(), nil)
	if nil != err {
		return prc, err
	}
	cf = append(cf, c[0].GetFilename())

	prc.ChangedFiles = cf

	response, err := http.Get(pr.GetDiffURL())
	if nil != err {
		return prc, err
	}

	//fetch content from diff url
	data, err := ioutil.ReadAll(response.Body)
	if nil != err {
		return prc, err
	}
	prc.Content = string(data)

	return prc, nil
}
