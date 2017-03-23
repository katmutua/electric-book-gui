package git

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/golang/glog"
	"github.com/google/go-github/github"

	"ebw/config"
	"ebw/util"
)

func ListPullRequests(client *Client, user, repo string) ([]*github.PullRequest, error) {
	requests, _, err := client.PullRequests.List(client.Context, user, repo, &github.PullRequestListOptions{})
	if nil != err {
		return nil, util.Error(err)
	}
	return requests, nil
}

func GetPullRequest(client *Client, user, repo string, number int) (*github.PullRequest, error) {
	pr, _, err := client.PullRequests.Get(client.Context, user, repo, number)
	return pr, err
}

// PullRequestDir returns the local git_cache location for the given pull request, or
// if sha is an empty string, the root directory for all prrequest checkouts
func PullRequestDir(sha string) (string, error) {
	root, err := os.Getwd()
	if nil != err {
		return ``, util.Error(err)
	}
	prRoot := filepath.Join(root, config.Config.GitCache, `pr_requests`)
	if `` == sha {
		return prRoot, nil
	}
	return filepath.Join(prRoot, sha), nil
}

// PullRequestCheckout checks out the given remoteUrl with given sha
func PullRequestCheckout(remoteUrl, sha string) (string, error) {
	glog.Infof(`PullRequestCheckout(remote = %s, sha = %s)`, remoteUrl, sha)
	prRoot, err := PullRequestDir(``)
	os.MkdirAll(prRoot, 0755)
	_, err = os.Stat(filepath.Join(prRoot, sha))
	if nil == err {
		prRoot = filepath.Join(prRoot, sha)
		// Update from origin / master
		if err = runGitDir(prRoot, []string{`pull`, `origin`, `master`}); nil != err {
			return ``, err
		}
	} else {
		if !os.IsNotExist(err) {
			return ``, util.Error(err)
		}
		glog.Infof("Going to clone %s", remoteUrl)
		if err = runGitDir(prRoot, []string{`clone`, remoteUrl, sha}); nil != err {
			return ``, err
		}
		prRoot = filepath.Join(prRoot, sha)
	}

	if err = runGitDir(prRoot, []string{`checkout`, sha}); nil != err {
		return ``, err
	}
	return prRoot, nil
}

func PullRequestDiffList(client *Client, user, repo string,
	sha string, pathRegexp string) ([]*PullRequestDiff, error) {
	localPath, err := RepoDir(user, repo)
	if nil != err {
		return nil, err
	}
	remotePath, err := PullRequestDir(sha)
	if nil != err {
		return nil, err
	}
	diffs, err := GetPathDiffList(localPath, remotePath, pathRegexp)
	return diffs, err
}

// PullRequestUpdate just updates the file in the 'master' repo the
// same as editing in the regular system.
func PullRequestUpdate(client *Client, user, repo string,
	sha string, path string, content []byte) error {
	return UpdateFile(client, user, repo, path, content)
	// localPath, err := RepoDir(user, repo)
	// if nil != err {
	// 	return err
	// }
	// if err := ioutil.WriteFile(filepath.Join(localPath, path), content, 0644); nil != err {
	// 	return util.Error(err)
	// }
	// return nil
}

func PullRequestClose(client *Client, user, repo string, number int) error {
	closedAt := time.Now()
	merged := true
	state := `closed`
	_, _, err := client.PullRequests.Edit(client.Context, user, repo, number, &github.PullRequest{
		Number:   &number,
		ClosedAt: &closedAt,
		Merged:   &merged,
		State:    &state,
	})
	if nil != err {
		return util.Error(err)
	}
	return nil
}

func PullRequestCreate(client *Client, user, repo, title, notes string) error {
	// base := `master`
	head := fmt.Sprintf(`%s:master`, user)
	// head := `master`
	base := `master`

	upstream, _, err := client.Repositories.Get(client.Context, user, repo)
	if nil != err {
		return err
	}
	upstreamUser := *upstream.Parent.Owner.Login
	upstreamRepo := *upstream.Parent.Name

	glog.Infof(`Creating new PR: title=%s, Head=%s, Base=%s, Body=%s, User=%s, Repo=%s`,
		title, head, base, notes, upstreamUser, upstreamRepo)
	_, _, err = client.PullRequests.Create(client.Context,
		upstreamUser, upstreamRepo,
		&github.NewPullRequest{
			Title: &title,
			Head:  &head,
			Base:  &base,
			// Body:  &notes,
		})
	if nil != err {
		return util.Error(err)
	}
	// _, _, err := client.PullRequests.CreateComment(user,
	// 	repo, prNumber, &github.PullRequestComment{
	// 		Comment: &notes,
	// 	})
	return nil
}

func GithubCreatePullRequest(
	client *Client,
	workingDir string,
	remote string,
	upstreamBranch string,
	title, notes string) error {
	var err error
	if `` == workingDir {
		workingDir, err = os.Getwd()
		if nil != err {
			return util.Error(err)
		}
	}

	localBranch, err := GitCurrentBranch(client, workingDir)
	if nil != err {
		return util.Error(err)
	}

	sourceHead := fmt.Sprintf(`%s:%s`, client.Username, localBranch)

	// remote will default to 'origin'
	_, remoteRepo, err := GitRemoteRepo(workingDir, remote)

	upstream, _, err := client.Repositories.Get(client.Context,
		client.Username, remoteRepo)
	if nil != err {
		return err
	}
	upstreamUser := *upstream.Parent.Owner.Login
	upstreamRepo := *upstream.Parent.Name

	glog.Infof(`Creating new PR: title=%s, Head=%s, Base=%s, Body=%s, User=%s, Repo=%s`,
		title, sourceHead, upstreamBranch, notes, upstreamUser, upstreamRepo)
	pr, _, err := client.PullRequests.Create(client.Context,
		upstreamUser, upstreamRepo,
		&github.NewPullRequest{
			Title: &title,
			Head:  &sourceHead,
			Base:  &upstreamBranch,
			Body:  &notes,
		})
	if nil != err {
		return util.Error(err)
	}
	fmt.Printf("Created PR %d on %s/%s\n", *pr.Number, upstreamUser, upstreamRepo)
	return nil
}
