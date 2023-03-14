# How to Contribute

We'd love to accept your patches and contributions to this project.

## Before you begin

### Sign our Contributor License Agreement

Contributions to this project must be accompanied by a
[Contributor License Agreement][cla] (CLA). You (or your employer) retain the
copyright to your contribution; this simply gives us permission to use and
redistribute your contributions as part of the project.

If you or your current employer have already signed the Google CLA (even if it
was for a different project), you probably don't need to do it again.

Visit <https://cla.developers.google.com/> to see your current agreements or to
sign a new one.

### Review our Community Guidelines

This project follows [Google's Open Source Community
Guidelines](https://opensource.google/conduct/).

## Contribution process

We follow the [fork and branch workflow][workflow].

There will be three Git repositories involved:

1.  *upstream* - the ServiceWeaver repository on GitHub.
2.  *origin* - your GitHub fork of `upstream`. This repository
    will typically be at a URL that looks like `github.com/_your_user_name_/weaver`
3.  *local* - your local clone of `origin`

Here is a detailed outline of the steps needed to make changes to Service
Weaver. Steps 1-3 will only be needed the first time you are making changes to
ServiceWeaver.

1.  Fork the `ServiceWeaver` repository on GitHub to create `origin`.
    Visit [ServiceWeaver][github_weaver] GitHub repository and click the `Fork` button.

2.  Make a `local` clone of your fork.

    ```shell
    git clone git@github.com:_your_user_name_/weaver.git
    ```

3.  Add a remote pointing from `local` to `upstream`.

    ```shell
    cd weaver
    git remote add upstream git@github.com:ServiceWeaver/weaver.git
    ```

4. Make a local branch in your clone and pull any recent changes into it.

   ```shell
   git switch -c my_branch  # Pick a name appropriate to your work
   git pull upstream main
   ```

5. Make changes and commit to local branch.

   ```shell
   # ... editing, testing, ... 
   git commit ...
   ```

   If you need to make multiple commits (e.g., if you checkpoint as
   you work), consider using `git commit --amend` or some other git
   command to keep all of your changes in a single commit.
    
   TODO: should we avoid discouraging multi-commit PRs?

6. Pull any changes that may have been made in the upstream repository
   main branch.

   ```shell
   git switch my_branch
   git pull --rebase upstream main
   ```

   Note that this command may result in merge conflicts. Fix those if
   needed.

7. Push your branch to the corresponding branch in your fork (the `origin` repository).

   ```shell
   git switch my_branch
   git push origin my_branch
   ```

8. Select the branch you are working on in the drop-down menu of branches on
   https://github.com/_your_user_name_/weaver . Then hit the `Compare and pull
   request` button.

9. Respond to feedback, which may involve making new commits or
   updating your prior commits. If you made any changes, push them
   to github again. If you amended a commit, you will have to force
   the push.

   ```shell
   git switch my_branch
   git push --force origin my_branch
   ```

10. Once reviewers are happy, pull any main branch changes that may
    have happened since step 6.
   
    ```shell
    git switch my_branch
    git pull --rebase upstream main
    ```

    If you made multiple commits, squash them together if you wish
    (typically using an interactive rebase).

    If you picked up new changes or made any changes, push your branch
    again to github, as described in step 9.

11. Ask somebody who has permissions (or do it yourself if you
    have permissions) to merge your branch into the main branch
    of the `upstream` repository. The reviewer may do this without
    being asked.

    Select the `Rebase and merge` option on https://github.com/ServiceWeaver/weaver
    or use the command line instructions found on that page.

12. Delete the branch from `local`

13. Delete the branch from `origin`: Visit https://github.com/_your_user_name/weaver/branches
    and check that `my_branch` is not marked as Open. If so, hit the delete
    button (looks like a trash icon) next to `my_branch`.

### Code Reviews

All submissions, including submissions by project members, require review. We
use GitHub pull requests for this purpose. Consult [GitHub Help][github_help]
for more information on using pull requests.

To make sure changes are well coordinated, we ask you to discuss any significant
change prior to sending a pull request. To do so, either file a
[new issue][new_issue] or claim an [existing one][issues].

## What to contribute to?

Here are the current areas where the community contributions would be most
valuable:

* Implement deployers for platforms other than local, SSH, and GKE.
* Bug fixes for the existing libraries.
* Non-Go language support. (Note: please reach out to us on our
  [google_group][Google Group], to coordinate development. We will likely
  require a fair bit of API and language-integration design discussion before
  the implementation starts.)

[cla]: https://cla.developers.google.com/about
[github_help]: https://help.github.com/articles/about-pull-requests/
[github_weaver]: https://github.com/ServiceWeaver/weaver
[google_group]: https://groups.google.com/g/serviceweaver
[issues]: https://github.com/ServiceWeaver/weaver/issues
[new_issue]: https://github.com/ServiceWeaver/weaver/issues/new
[workflow]: https://www.google.com/search?q=github+fork+and+branch+workflow
