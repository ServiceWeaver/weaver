#/usr/bin/env bash

# This file is a bash completion script that enables tab completion for the
# weaver command. For example:
#
#     $ weaver <tab><tab>
#     generate   gke        gke-local  multi      single     ssh        version
#     $ weaver single <tab><tab>
#     dashboard  deploy     help       metrics    profile    purge      status     version
#     $ weaver single p<tab><tab>
#     profile  purge
#
# To enable the tab completion, you must source this file, typically by running
# `source <(weaver completion)`. This mirrors `kubectl completion bash` [3].
# Source the file in your ~/.bashrc if you always want it to be enabled.
#
# See [1] for a tutorial on how to write bash completion scripts. See [2] for an
# explanation of our usage of `-o default`.
#
# [1]: https://opensource.com/article/18/3/creating-bash-completion-script
# [2]: https://stackoverflow.com/a/19062943
# [3]: https://kubernetes.io/docs/reference/kubectl/cheatsheet/#kubectl-autocomplete

_weaver_complete() {
  compopt +o default

  # weaver
  if [[ $COMP_CWORD == 1 ]]; then
    COMPREPLY=($(compgen -W "generate version single multi ssh gke gke-local" "${COMP_WORDS[1]}"))
    return
  fi

  # weaver generate
  if [[ "${COMP_WORDS[1]}" == "generate" ]]; then
    compopt -o default
    return
  fi

  # weaver version
  if [[ "${COMP_WORDS[1]}" == "version" ]]; then
    return
  fi

  # weaver {single,multi,ssh,gke,gke-local}
  declare -A subcommands
  subcommands["single"]="dashboard deploy help metrics profile purge status version"
  subcommands["multi"]="dashboard deploy help logs metrics profile purge status version"
  subcommands["ssh"]="dashboard deploy help logs version"
  subcommands["gke"]="dashboard deploy help kill logs profile status version"
  subcommands["gke-local"]="dashboard deploy help kill logs profile purge status version"
  local -r deployer="${COMP_WORDS[1]}"
  if [[ "${subcommands[$deployer]}" != "" ]]; then
    return
  elif [[ $COMP_CWORD == 2 ]]; then
    COMPREPLY=($(compgen -W "${subcommands[$deployer]}" "${COMP_WORDS[2]}"))
  elif [[ ${COMP_WORDS[2]} == "deploy" && $COMP_CWORD == 3 ]]; then
    compopt -o default
  elif [[ ${COMP_WORDS[2]} == "help" && $COMP_CWORD == 3 ]]; then
    COMPREPLY=($(compgen -W "${subcommands[$deployer]}" "${COMP_WORDS[3]}"))
  fi
}

complete -o default -F _weaver_complete weaver
