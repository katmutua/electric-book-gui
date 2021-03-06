package git

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
	git2go "gopkg.in/libgit2/git2go.v25"

	"ebw/config"
	"ebw/util"
)

var ErrNoGitDirectory = errors.New(`Not a GIT directory`)

// ErrUnknownUser indicates that Github was unable to resolve the user's name.
var ErrUnknownUser = errors.New(`UnknownUser: no recognized login or ID`)

// Username returns the username of the currently logged in user
// on GitHub, retrieved from the Client.
func Username(client *Client) (string, error) {
	// Empty username gives the currently logged-in user
	user, _, err := client.Users.Get(client.Context, "")
	if nil != err {
		return ``, util.Error(err)
	}
	if nil != user.Login {
		return *user.Login, nil
	}
	if nil != user.ID {
		return strconv.FormatInt(int64(*user.ID), 10), nil
	}
	return ``, ErrUnknownUser
}

// RepoDir returns the local git_cache repo working directory location.
// If repoOwner or repoName is an empty string, the path is returned
// up to that level.
func RepoDir(user, repoOwner, repoName string) (string, error) {
	root, err := os.Getwd()
	if nil != err {
		return ``, util.Error(err)
	}
	root = filepath.Join(root, config.Config.GitCache, `repos`, user)
	if `` == repoOwner {
		return root, nil
	}
	if `` == repoName {
		return filepath.Join(root, repoOwner), nil
	}
	return filepath.Join(root, repoOwner, repoName), nil
}

// Checkout checks out the github repo into the cached directory system,
// and returns the path to the root of the repo. If the client is already
// checked out, it updates from the origin server.
func Checkout(client *Client, repoOwner, repoName, repoUrl string) (string, error) {
	if `` == repoUrl {
		repoUrl = fmt.Sprintf(`https://%s:%s@github.com/%s/%s`,
			client.Username, client.Token, repoOwner, repoName)
	} else {
		ux, err := url.Parse(repoUrl)
		if nil != err {
			return ``, util.Error(err)
		}
		if nil == ux.User || ux.User.Username() == `` {
			ux.User = url.UserPassword(client.Username, client.Token)
		}
		repoUrl = ux.String()
	}
	glog.Infof(`Cloning/updating %s/%s from %s`, repoOwner, repoName, repoUrl)
	// We cannot create the directory for the repo if we are going
	// to clone - that will cause a 'directory already exists' error.
	// So we only create the parent directory, then determine whether
	// the repoDirectory exists or not.
	repoOwnerDir, err := RepoDir(client.Username, repoOwner, ``)
	if nil != err {
		return ``, util.Error(err)
	}
	repoDir := filepath.Join(repoOwnerDir, repoName)
	os.MkdirAll(repoOwnerDir, 0755)
	_, err = os.Stat(repoDir)
	if nil == err {
		return gitUpdate(client, repoDir)
	}
	if !os.IsNotExist(err) {
		return ``, util.Error(err)
	}

	cmd := exec.Command(`git`, `clone`, repoUrl+`.git`)
	cmd.Dir = repoOwnerDir
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); nil != err {
		return ``, util.Error(err)
	}

	return repoDir, gitConfig(client, repoDir)
}

// gitConfig configures the git username and email for the given
// client-repo combination.
func gitConfig(client *Client, repoDir string) error {
	// repoDir, err := RepoDir(client.Username, repoOwner, repoName)
	// if nil != err {
	// 	return util.Error(err)
	// }
	if err := runGitDir(repoDir, []string{`config`, `user.name`, client.Username}); nil != err {
		return util.Error(err)
	}
	if nil != client.User {
		if err := runGitDir(repoDir, []string{`config`, `user.email`, client.User.GetEmail()}); nil != err {
			return util.Error(err)
		}
	} else {
		glog.Errorf(`Unable to set user.email for user %s in %s: no Email set`, client.Username, repoDir)
	}
	return nil
}

func Commit(client *Client, repoOwner, repoName, message string) (*git2go.Oid, error) {
	repoDir, err := RepoDir(client.Username, repoOwner, repoName)
	if nil != err {
		return nil, err
	}

	// We are using git2go to do the commit
	// if err = runGitDir(repoDir, []string{`commit`, `-am`, message}); nil != err {
	// 	return err
	// }
	repo, err := git2go.OpenRepository(repoDir)
	if nil != err {
		return nil, util.Error(err)
	}
	defer repo.Free()
	author := &git2go.Signature{
		Name:  client.Username,
		Email: client.User.GetEmail(),
		When:  time.Now(),
	}
	// TODO: If we don't have a User Email address,
	// where can we get one?
	if `` == author.Email {
		author.Email = author.Name
	}
	glog.Infof(`Committing with signatures Name:%s, Email:%s`, client.Username, client.User.GetEmail())
	index, err := repo.Index()
	if nil != err {
		return nil, util.Error(err)
	}
	defer index.Free()
	treeId, err := index.WriteTree()
	if nil != err {
		return nil, util.Error(err)
	}
	tree, err := repo.LookupTree(treeId)
	if nil != err {
		return nil, util.Error(err)
	}
	defer tree.Free()

	//Getting repo HEAD
	head, err := repo.Head()
	if err != nil {
		return nil, util.Error(err)
	}
	defer head.Free()

	headCommit, err := repo.LookupCommit(head.Target())
	if err != nil {
		return nil, util.Error(err)
	}
	defer headCommit.Free()

	oid, err := repo.CreateCommit(`HEAD`, author, author, message, tree, headCommit)
	if nil != err {
		return nil, util.Error(err)
	}
	glog.Infof(`COMMIT Created: oid = %s`, oid.String())

	// Push the server-side commit to our master: which is probably
	// our FORK of a repo.
	if err = runGitDir(repoDir, []string{`push`, `origin`, `master`}); nil != err {
		return nil, err
	}
	return oid, nil
}

// gitUpdate updates the files in the given repo root directory.
func gitUpdate(client *Client, root string) (string, error) {
	cmd := exec.Command(`git`, `pull`, `origin`, `master`)
	cmd.Dir = root
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	glog.Infof("dir = %s: git pull origin master", root)
	if err := cmd.Run(); nil != err {
		return ``, util.Error(err)
	}
	return root, gitConfig(client, root)
}

// RemoteName returns a name for the remote based on the remoteUrl
func RemoteName(remoteUrl string) string {
	return fmt.Sprintf(`%x`, md5.Sum([]byte(remoteUrl)))
}

// RemoteAdd adds a new Remote to the git remotes
func RemoteAdd(client *Client, user, repoOwner, repoName, remoteUrl string) (string, error) {
	remote := RemoteName(remoteUrl)
	return remote, runGit(user, repoOwner, repoName, []string{`remote`, `add`, remote, remoteUrl})
}

// UrlUserRepo returns the user and repo given a github URL
func UrlUserRepo(remoteUrl string) (string, string, error) {
	reg := regexp.MustCompile(`github.com/([^/]+)/([^/]+)`)
	if m := reg.FindStringSubmatch(remoteUrl); nil != m {
		return m[1], m[2], nil
	}
	return ``, ``, fmt.Errorf(`repo %s is not a github repo`, remoteUrl)
}

// PullRequestVersions returns the local and remote version of the file
// named in filePath. This is used to match files for pull-request file
// merging.
func PullRequestVersions(client *Client, user, repoOwner, repoName, remoteUrl, remoteSha, filePath string) (string, string, error) {
	// We are sure this exists because of the point at which we call
	// this from the JS front-end

	// prRoot, err := PullRequestCheckout(remoteUrl, remoteSha)
	// if nil != err {
	// 	return ``, ``, err
	// }
	prRoot, err := PullRequestDir(remoteSha)
	if nil != err {
		return ``, ``, err
	}

	repoDir, err := RepoDir(user, repoOwner, repoName)
	if nil != err {
		return ``, ``, err
	}

	localFileRaw, err := ioutil.ReadFile(filepath.Join(repoDir, filePath))
	if nil != err {
		if !os.IsNotExist(err) {
			return ``, ``, util.Error(err)
		}
		localFileRaw = []byte{}
	}

	remoteFileRaw, err := ioutil.ReadFile(filepath.Join(prRoot, filePath))
	if nil != err {
		if !os.IsNotExist(err) {
			return ``, ``, util.Error(err)
		}
		remoteFileRaw = []byte{}
	}

	return string(localFileRaw), string(remoteFileRaw), nil
}

// DuplicateRepo duplicates the template repo into the user's github repos,
//  and gives it the name newRepo.
// This is used to start a new book, without being a fork of
// the EBW electric-book repo.
// See https://help.github.com/articles/duplicating-a-repository/
// for more information.
func DuplicateRepo(client *Client, githubPassword string, templateRepo string, newRepo string) error {
	repoName := filepath.Base(newRepo)
	// 1. Check the user doesn't already have a newRepo,
	//  and if not, create a newRepo for the user
	workingDir := filepath.Join(os.TempDir(), client.Username, newRepo)
	os.MkdirAll(workingDir, 0755)

	_, _, err := client.Repositories.Create(client.Context, "", &github.Repository{
		Name:  &newRepo,
		Owner: client.User,
	})
	if nil != err {
		return util.Error(err)
	}

	glog.Infof(`Going to fork repo %s into %s/%s`, templateRepo, client.Username, newRepo)

	// 2. Checkout the templateRepo with --bare into a new directory called [repoName]
	if err := runGitDir(workingDir, []string{
		`clone`,
		`--bare`,
		// `--depth`, `1`,
		`https://` + client.Username + `:` + githubPassword + `@github.com/` + templateRepo + `.git`,
		repoName,
	}); nil != err {
		return util.Error(err)
	}

	// 3. Mirror-push to the newRepo
	if err := runGitDir(filepath.Join(workingDir, repoName), []string{
		`push`, `--mirror`, `https://` + client.Username + `:` + githubPassword + `@github.com/` + client.Username + `/` + repoName + `.git`,
	}); nil != err {
		return util.Error(err)
	}
	// 4. Delete the temporary working directory
	if err := os.RemoveAll(filepath.Join(workingDir, repoName)); nil != err {
		return util.Error(err)
	}
	return nil
}

func ContributeToRepo(client *Client, repoUserAndName string) error {
	// See CLI BookContribute for model of how this should function.
	parts := strings.Split(repoUserAndName, `/`)
	if 2 != len(parts) {
		return errors.New(`repo should be user/repo format`)
	}
	_, _, err := client.Repositories.CreateFork(
		client.Context,
		parts[0],
		parts[1],
		&github.RepositoryCreateForkOptions{})
	if nil != err {
		return err
	}

	repoDir, err := RepoDir(client.Username, parts[0], parts[1])
	if nil != err {
		return err
	}
	return GitCloneTo(client, repoDir, /* empty working dir will default to current dir */
		parts[0], parts[1])
}

func GitCloneTo(client *Client, workingDir string, repoUsername, repoName string) error {
	if "" == workingDir {
		wd, err := os.Getwd()
		if nil != err {
			return util.Error(err)
		}
		workingDir = wd
	}

	if "" == repoUsername {
		repoUsername = client.Username
	}

	if err := runGitDir(workingDir, []string{
		`clone`,
		`https://` + client.Username + ":" + client.Token +
			"@github.com/" + repoUsername + "/" + repoName + ".git",
	}); nil != err {
		return util.Error(err)
	}
	return nil
}

// GithubDeleteRepo deletes a repository on the
// github systems.
// See https://developer.github.com/v3/repos/#delete-a-repository
// Note that this should be used with extreme caution, since this is a
// total delete of the repo and all it contains, and cannot be undone.
func GithubDeleteRepo(apiToken string, githubUsername string,
	repoName string) error {
	requestUrl := `https://api.github.com/repos/` + githubUsername + `/` + repoName
	req, err := http.NewRequest(`DELETE`,
		requestUrl, nil)
	if nil != err {
		return util.Error(err)
	}
	req.Header.Add(`Authorization`, `token `+apiToken)
	client := &http.Client{}
	res, err := client.Do(req)
	if nil != err {
		return util.Error(err)
	}
	defer res.Body.Close()
	if 200 > res.StatusCode || 300 <= res.StatusCode {
		fmt.Printf(`Command is: 
curl -v -X DELETE -H "Authorization: token %s" '%s'
`, apiToken, requestUrl)
		return fmt.Errorf(`Bad status code result: %d (ensure your token has repo_delete privileges)`, res.StatusCode)
	}
	io.Copy(os.Stdout, res.Body)
	return nil
}

// GitRemoteRepo returns the remote repo name of the given remote
// It expects a remote of the form:
// [remotename] [remoteURL] ([fetch|push)])
// so parses the results of `git remote get-url [remote]`
// which is expected to be a URL, takes the path and strips .git
func GitRemoteRepo(workingDir, remote string) (remoteUser, remoteProject string, err error) {
	if `` == remote {
		remote = `origin`
	}
	repo, err := git2go.OpenRepository(util.WorkingDir(workingDir))
	if nil != err {
		return ``, ``, util.Error(err)
	}
	defer repo.Free()
	rem, err := repo.Remotes.Lookup(remote)
	if nil != err {
		glog.Error(err)
		return ``, ``, err
	}
	defer rem.Free()
	return gitRemoteParseUrl(rem.Url())
}

// DEPRECATED - we're using git2go instead
// func GitRemoteRepo_shell(workingDir, remote string) (remoteUser, remoteProject string, err error) {
// 	if `` == remote {
// 		remote = `origin`
// 	}
// 	if `` == workingDir {
// 		workingDir, err = os.Getwd()
// 		if nil != err {
// 			return ``, ``, util.Error(err)
// 		}
// 	}

// 	remoteUrl, err := getGitOutput(workingDir, []string{
// 		`remote`,
// 		`get-url`,
// 		remote,
// 	})
// 	if nil != err {
// 		return ``, ``, err
// 	}
// 	return gitRemoteParseUrl(remoteUrl)
// }

func gitRemoteParseUrl(remoteUrl string) (string, string, error) {
	ru, err := url.Parse(remoteUrl)
	if nil != err {
		return ``, ``, util.Error(err)
	}
	path := strings.TrimPrefix(strings.TrimSpace(ru.Path), `/`)
	if strings.HasSuffix(path, `.git`) {
		path = path[0 : len(path)-4]
	}
	paths := strings.Split(path, `/`)
	remoteProject := paths[len(paths)-1]
	remoteUser := strings.Join(paths[0:len(paths)-1], `/`)

	return remoteUser, remoteProject, nil
}

// GitCurrentBranch returns the name of the currently checked-out
// branch.
func GitCurrentBranch(clinet *Client, workingDir string) (string, error) {
	branchesOut, err := getGitOutput(workingDir, []string{
		`branch`, `--list`,
	})
	if nil != err {
		return ``, err
	}
	branches := strings.Split(branchesOut, "\n")
	for _, b := range branches {
		if 0 == len(b) {
			continue
		}
		// Current branch indicated with an asterisk
		if b[0] == '*' {
			return strings.TrimSpace(b[1:]), nil
		}
	}
	return ``, errors.New(`No current branch`)
}

// GitFindRepoRootDirectory returns the first parent directory containing
// a .git subfolder, or an error if no such directory is found.
func GitFindRepoRootDirectory(workingDir string) (string, error) {
	var err error
	if `` == workingDir {
		workingDir, err = os.Getwd()
		if nil != err {
			return ``, util.Error(err)
		}
	}
	_, err = os.Stat(filepath.Join(workingDir, `.git`))
	if nil == err {
		return workingDir, nil
	}
	if !os.IsNotExist(err) {
		return ``, util.Error(err)
	}
	// .git directory doesn't exist in this directory
	parent := filepath.Dir(workingDir)
	if 0 == len(parent) || workingDir == parent {
		return ``, ErrNoGitDirectory
	}
	return GitFindRepoRootDirectory(parent)
}

type StatusList struct {
	*git2go.StatusList
}
type StatusEntry struct {
	git2go.StatusEntry
}

// Filename returns the name of the file based on the
// HEAD-Index status.
func (se *StatusEntry) Filename() string {
	if 0 != se.Status&(git2go.StatusIndexNew|git2go.StatusIndexModified) {
		return se.HeadToIndex.NewFile.Path
	}
	if 0 != se.Status&(git2go.StatusIndexDeleted) {
		return se.HeadToIndex.OldFile.Path
	}
	if 0 != se.Status&(git2go.StatusIndexRenamed) {
		return se.HeadToIndex.OldFile.Path + " renamed to " + se.HeadToIndex.NewFile.Path
	}
	return fmt.Sprintf(`Currently unsupported status %v`, se.Status)
}

// StatusType returns a textual type description of the
// HEAD-Index status of the entry
func (se *StatusEntry) StatusType() string {
	switch {
	case 0 != se.Status&git2go.StatusIndexNew:
		return "new"
	case 0 != se.Status&git2go.StatusIndexModified:
		return "modified"
	case 0 != se.Status&git2go.StatusIndexDeleted:
		return "deleted"
	case 0 != se.Status&git2go.StatusIndexRenamed:
		return "renamed"
	}
	return "unsupported"
}

func (s *StatusList) Statuses() chan *StatusEntry {
	C := make(chan *StatusEntry)
	count, err := s.EntryCount()
	if nil != err {
		panic(err)
	}
	go func() {
		defer close(C)
		for i := 0; i < count; i++ {
			se, err := s.ByIndex(i)
			if nil != err {
				panic(err)
			}
			C <- &StatusEntry{se}
		}
	}()
	return C
}

// GitStatusList returns the status list for a particular repo. If
// the caller provides a Context, the StatusList will be freed when
// the Context is done. Otherhe
// caller MUST call .Free on the StatusList when done.
func GitStatusList(ctxt context.Context, repoDir string) (*StatusList, error) {
	repo, err := git2go.OpenRepository(repoDir)
	if nil != err {
		return nil, util.Error(err)
	}
	defer repo.Free()
	sl, err := repo.StatusList(&git2go.StatusOptions{
		Show: git2go.StatusShowIndexOnly,
	})
	if nil != err {
		return nil, util.Error(err)
	}
	if nil != ctxt {
		c, _ := context.WithCancel(ctxt)
		go func() {
			<-c.Done()
			sl.Free()
		}()
	}
	return &StatusList{sl}, nil
}
