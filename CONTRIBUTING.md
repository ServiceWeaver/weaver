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

Here is a detailed outline of the steps needed to make changes to Service
Weaver.

1. Clone the repository.

   ```shell
   git clone git@github.com:ServiceWeaver/weaver.git
   cd weaver
   ```

   If you have already cloned the repository for previous changes,
   you can reuse it, but make sure to first pull any recent changes
   to Service Weaver:

   ```shell
   cd weaver
   git switch main
   git pull origin main
   ```

2. Make a local branch in your clone.

   ```shell
   git switch -c my_branch  # Pick a name appropriate to your work
   ```

3. Make changes and commit to local branch.

   ```shell
   # ... editing, testing, ... 
   git commit ...
   ```

   If you need to make multiple commits (e.g., if you checkpoint as
   you work), consider using `git commit --amend` or some other git
   command to keep all of your changes in a single commit.
    
   TODO: should we avoid discouraging multi-commit PRs?

4. Pull any changes that may have been made in the shared repository
   main branch.

   ```shell
   git switch my_branch
   git pull --rebase origin main
   ```

   Note that this command may result in merge conflicts. Fix those if
   needed.

5. Push your branch to the corresponding branch in the shared repository.

   ```shell
   git switch my_branch
   git push -u origin my_branch
   ```

6. Select the branch you are working on in the drop-down menu of branches on
   https://github.com/ServiceWeaver/weaver . Then hit the `Compare and pull
   request` button.

7. Respond to feedback, which may involve making new commits or
   updating your prior commits. If you made any changes, push them
   to github again. If you amended a commit, you will have to force
   the push.

   ```shell
   git switch my_branch
   git push --force origin my_branch
   ```

8. Once reviewers are happy, pull any main branch changes that may
   have happened since step 4.
   
   ```shell
   git switch my_branch
   git pull --rebase origin main
   ```

   If you made multiple commits, squash them together if you wish
   (typically using an interactive rebase).

   If you picked up new changes or made any changes, push your branch
   again to github, as described in step 7.

9. Ask somebody who has permissions (or do it yourself if you
   have permissions) to merge your branch into the main branch
   of the shared repository. The reviewer may do this without
   being asked.

   Select the `Rebase and merge` option on https://github.com/ServiceWeaver/weaver
   or use the command line instructions found on that page.

10. Delete the branch: Visit https://github.com/ServiceWeaver/weaver/branches
    and check that `my_branch` is not marked as Open. If so, hit the delete
    button (looks like a trash icon) next to `my_branch`.

### Code Reviews

All submissions, including submissions by project members, require review. We
use GitHub pull requests for this purpose. Consult [GitHub Help][github_help]
for more information on using pull requests.

To make sure changes are well coordinated, we ask you to discuss any significant
change prior to sending a pull request. To do so, either file a
[new issue][new_issue] or claim an [existing one][issues].

[cla]: https://cla.developers.google.com/about
[github_help]: https://help.github.com/articles/about-pull-requests/
[new_issue]: https://github.com/ServiceWeaver/weaver/issues/new
[issues]: https://github.com/ServiceWeaver/weaver/issues
