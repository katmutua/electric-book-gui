package git

import (
	"github.com/google/go-github/github"
	"github.com/golang/glog"
	"log"
	//"gopkg.in/libgit2/git2go.v25"
)

//  need to sum the effect of
//    `git log --walk-reflogs <branch_name>`
//  for each branch in a repo branch list


func TotalCommits(client *Client, gr *GitRepo) (int, error) {
	//fetch all the branches
	branchList, err := GetAllBranches(client, gr.GetName())

	if nil != err {
		log.Panic(err)
	}

	var commitSum int
	//fetch all commits starting from a certain branch
	for _, br := range branchList {
		commitOptions := &github.CommitsListOptions{
			SHA: br.Commit.GetSHA(),
		}
		commits, _, err := client.Repositories.ListCommits(client.Context,
			client.Username, gr.GetName(), commitOptions)

		if nil != err {
		}
		commitSum += len(commits)
	}

	return commitSum, nil
}


func GetAllBranches(client *Client, repoName string) ([]*github.Branch, error) {
	branchList, _, err := client.Repositories.ListBranches(client.Context,
		client.Username, repoName, nil)

	if nil != err {
		glog.Errorf(`Error on getallbranches(%s): %s`, repoName, err.Error())
	}
	return branchList, nil
}