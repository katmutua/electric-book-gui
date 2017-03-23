package git

import (
	"bytes"
	"os"
	"os/exec"

	"github.com/golang/glog"

	"ebw/util"
)

// runGit runs git in the user/repo cache working directory with
// the given args, returning error
// on failure.
func runGit(user, repo string, args []string) error {
	root, err := RepoDir(user, repo)
	if nil != err {
		return err
	}
	return runGitDir(root, args)
}

// runGitDir runs git in the given directory with the given arguments.
func runGitDir(dir string, args []string) error {
	glog.Infof(`git command dir=%s, args = [%v]`, dir, args)
	cmd := exec.Command(`git`, args...)
	cmd.Dir = dir

	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// getGitOutput runs the git command in the given directory
// and returns stdout as a string
func getGitOutput(dir string, args []string) (string, error) {
	if `` == dir {
		var err error
		dir, err = os.Getwd()
		if nil != err {
			return ``, util.Error(err)
		}
	}
	glog.Infof(`git command dir=%s, args = [%v]`, dir, args)
	cmd := exec.Command(`git`, args...)
	cmd.Dir = dir

	var stdOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdOut, os.Stderr

	if err := cmd.Run(); nil != err {
		return ``, util.Error(err)
	}
	return stdOut.String(), nil
}
