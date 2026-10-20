# bash completion for gputop
#
# Installed as /usr/share/bash-completion/completions/gputop and sourced by
# bash-completion. The option list is the one cmd/gputop/main.go registers;
# scripts/check_packaging.sh fails when the two drift apart.

_gputop() {
    local cur prev
    if declare -F _init_completion >/dev/null; then
        _init_completion || return
    else
        cur="${COMP_WORDS[COMP_CWORD]}"
        prev="${COMP_WORDS[COMP_CWORD-1]}"
    fi

    case "${prev}" in
        -c|--config|--log)
            if declare -F _filedir >/dev/null; then
                _filedir
            else
                COMPREPLY=( $(compgen -f -- "${cur}") )
            fi
            return
            ;;
        --drm-root|--proc-root)
            if declare -F _filedir >/dev/null; then
                _filedir -d
            else
                COMPREPLY=( $(compgen -d -- "${cur}") )
            fi
            return
            ;;
        -i|--interval)
            # A number of seconds: nothing sensible to suggest.
            return
            ;;
        --kind)
            COMPREPLY=( $(compgen -W "auto igpu dgpu" -- "${cur}") )
            return
            ;;
        --theme)
            COMPREPLY=( $(compgen -W "default dracula gruvbox monochrome radeon" -- "${cur}") )
            return
            ;;
    esac

    local opts="-c -i --config --interval --drm-root --proc-root --dump --pretty"
    opts+=" --devices --kind --theme --no-color --no-processes --blocks --no-blocks"
    opts+=" --log --check --json --version"
    if [[ "${cur}" == -* ]]; then
        COMPREPLY=( $(compgen -W "${opts}" -- "${cur}") )
        return
    fi
    COMPREPLY=()
}

complete -F _gputop gputop
