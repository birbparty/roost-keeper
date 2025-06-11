package cli

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

// CompletionCommand returns the completion command
func CompletionCommand() *cli.Command {
	return &cli.Command{
		Name:  "completion",
		Usage: "Generate shell completion scripts",
		Description: `Generate shell completion scripts for roost CLI.

To load bash completions:
  source <(roost completion bash)

To load zsh completions:
  roost completion zsh > _roost
  # Move _roost to your zsh completions directory

To load fish completions:
  roost completion fish | source

To load PowerShell completions:
  roost completion powershell | Out-String | Invoke-Expression`,
		Subcommands: []*cli.Command{
			{
				Name:  "bash",
				Usage: "Generate bash completion script",
				Action: func(c *cli.Context) error {
					return generateBashCompletion()
				},
			},
			{
				Name:  "zsh",
				Usage: "Generate zsh completion script",
				Action: func(c *cli.Context) error {
					return generateZshCompletion()
				},
			},
			{
				Name:  "fish",
				Usage: "Generate fish completion script",
				Action: func(c *cli.Context) error {
					return generateFishCompletion()
				},
			},
			{
				Name:  "powershell",
				Usage: "Generate PowerShell completion script",
				Action: func(c *cli.Context) error {
					return generatePowerShellCompletion()
				},
			},
		},
	}
}

// generateBashCompletion generates bash completion script
func generateBashCompletion() error {
	script := `#!/bin/bash

_roost_completion() {
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    # Main commands
    if [[ ${COMP_CWORD} == 1 ]]; then
        opts="get create apply delete describe status logs health template completion version"
        COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
        return 0
    fi

    # Subcommands
    case "${COMP_WORDS[1]}" in
        get|describe|status|logs)
            if [[ ${COMP_CWORD} == 2 ]]; then
                # Complete with ManagedRoost names
                opts=$(kubectl get managedroosts -o name 2>/dev/null | cut -d'/' -f2)
                COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
            fi
            ;;
        create|apply|delete)
            case "$prev" in
                -f|--filename)
                    COMPREPLY=( $(compgen -f -- ${cur}) )
                    ;;
                *)
                    opts="-f --filename --dry-run"
                    COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
                    ;;
            esac
            ;;
        health)
            if [[ ${COMP_CWORD} == 2 ]]; then
                opts="check list debug"
                COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
            elif [[ ${COMP_CWORD} == 3 ]] && [[ "${COMP_WORDS[2]}" =~ ^(check|list|debug)$ ]]; then
                # Complete with ManagedRoost names
                opts=$(kubectl get managedroosts -o name 2>/dev/null | cut -d'/' -f2)
                COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
            fi
            ;;
        template)
            if [[ ${COMP_CWORD} == 2 ]]; then
                opts="generate validate"
                COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
            elif [[ "${COMP_WORDS[2]}" == "generate" ]]; then
                case "$prev" in
                    --preset)
                        opts="simple complex monitoring webapp"
                        COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
                        ;;
                    --output|-o)
                        COMPREPLY=( $(compgen -f -- ${cur}) )
                        ;;
                esac
            elif [[ "${COMP_WORDS[2]}" == "validate" ]]; then
                case "$prev" in
                    -f|--filename)
                        COMPREPLY=( $(compgen -f -- ${cur}) )
                        ;;
                esac
            fi
            ;;
        completion)
            if [[ ${COMP_CWORD} == 2 ]]; then
                opts="bash zsh fish powershell"
                COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
            fi
            ;;
    esac

    # Global flags
    case "$prev" in
        -n|--namespace)
            opts=$(kubectl get namespaces -o name 2>/dev/null | cut -d'/' -f2)
            COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
            ;;
        -o|--output)
            opts="table yaml json wide"
            COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
            ;;
        --context)
            opts=$(kubectl config get-contexts -o name 2>/dev/null)
            COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
            ;;
    esac
}

complete -F _roost_completion roost
`

	fmt.Print(script)
	return nil
}

// generateZshCompletion generates zsh completion script
func generateZshCompletion() error {
	script := `#compdef roost

_roost() {
    local context curcontext="$curcontext" state line
    typeset -A opt_args

    _arguments -C \
        '(-h --help)'{-h,--help}'[show help]' \
        '(-v --version)'{-v,--version}'[show version]' \
        '--kubeconfig[path to kubeconfig file]:kubeconfig:_files' \
        '--context[kubernetes context]:context:_roost_contexts' \
        '(-n --namespace)'{-n,--namespace}'[kubernetes namespace]:namespace:_roost_namespaces' \
        '(-o --output)'{-o,--output}'[output format]:format:(table yaml json wide)' \
        '--verbose[enable verbose output]' \
        '--debug[enable debug logging]' \
        '1: :_roost_commands' \
        '*::arg:->args'

    case $state in
        args)
            case $words[1] in
                get|describe|status|logs)
                    _arguments \
                        '(-l --selector)'{-l,--selector}'[label selector]:selector:' \
                        '--all-namespaces[list resources from all namespaces]' \
                        '(-w --watch)'{-w,--watch}'[watch for changes]' \
                        '*:managedroost:_roost_managedroosts'
                    ;;
                create|apply|delete)
                    _arguments \
                        '(-f --filename)'{-f,--filename}'[YAML file]:file:_files -g "*.yaml" -g "*.yml"' \
                        '--dry-run[validate without applying]' \
                        '--force[force operation]'
                    ;;
                health)
                    case $words[2] in
                        check|list|debug)
                            _arguments '*:managedroost:_roost_managedroosts'
                            ;;
                        *)
                            _arguments '1:command:(check list debug)'
                            ;;
                    esac
                    ;;
                template)
                    case $words[2] in
                        generate)
                            _arguments \
                                '--chart[helm chart name]:chart:' \
                                '--repo[helm repository URL]:repo:' \
                                '--version[chart version]:version:' \
                                '(-n --namespace)'{-n,--namespace}'[target namespace]:namespace:_roost_namespaces' \
                                '--output[output file]:file:_files' \
                                '--preset[preset template]:preset:(simple complex monitoring webapp)'
                            ;;
                        validate)
                            _arguments \
                                '(-f --filename)'{-f,--filename}'[YAML file]:file:_files -g "*.yaml" -g "*.yml"'
                            ;;
                        *)
                            _arguments '1:command:(generate validate)'
                            ;;
                    esac
                    ;;
                completion)
                    _arguments '1:shell:(bash zsh fish powershell)'
                    ;;
            esac
            ;;
    esac
}

_roost_commands() {
    local commands=(
        'get:Get ManagedRoost resources'
        'create:Create ManagedRoost resources'
        'apply:Apply ManagedRoost configuration'
        'delete:Delete ManagedRoost resources'
        'describe:Describe ManagedRoost resources'
        'status:Show detailed status of ManagedRoost resources'
        'logs:Stream logs from ManagedRoost resources'
        'health:Health check operations'
        'template:Generate ManagedRoost templates'
        'completion:Generate shell completion scripts'
        'version:Show version information'
    )
    _describe 'commands' commands
}

_roost_managedroosts() {
    local managedroosts
    managedroosts=(${(f)"$(kubectl get managedroosts -o name 2>/dev/null | cut -d'/' -f2)"})
    _describe 'managedroosts' managedroosts
}

_roost_namespaces() {
    local namespaces
    namespaces=(${(f)"$(kubectl get namespaces -o name 2>/dev/null | cut -d'/' -f2)"})
    _describe 'namespaces' namespaces
}

_roost_contexts() {
    local contexts
    contexts=(${(f)"$(kubectl config get-contexts -o name 2>/dev/null)"})
    _describe 'contexts' contexts
}

_roost "$@"
`

	fmt.Print(script)
	return nil
}

// generateFishCompletion generates fish completion script
func generateFishCompletion() error {
	script := `# roost fish completion

function __fish_roost_needs_command
    set cmd (commandline -opc)
    if [ (count $cmd) -eq 1 ]
        return 0
    end
    return 1
end

function __fish_roost_using_command
    set cmd (commandline -opc)
    if [ (count $cmd) -gt 1 ]
        if [ $argv[1] = $cmd[2] ]
            return 0
        end
    end
    return 1
end

function __fish_roost_get_managedroosts
    kubectl get managedroosts -o name 2>/dev/null | cut -d'/' -f2
end

function __fish_roost_get_namespaces
    kubectl get namespaces -o name 2>/dev/null | cut -d'/' -f2
end

function __fish_roost_get_contexts
    kubectl config get-contexts -o name 2>/dev/null
end

# Global options
complete -c roost -s h -l help -d "Show help"
complete -c roost -s v -l version -d "Show version"
complete -c roost -l kubeconfig -d "Path to kubeconfig file" -F
complete -c roost -l context -d "Kubernetes context" -f -a "(__fish_roost_get_contexts)"
complete -c roost -s n -l namespace -d "Kubernetes namespace" -f -a "(__fish_roost_get_namespaces)"
complete -c roost -s o -l output -d "Output format" -f -a "table yaml json wide"
complete -c roost -l verbose -d "Enable verbose output"
complete -c roost -l debug -d "Enable debug logging"

# Main commands
complete -c roost -f -n "__fish_roost_needs_command" -a "get" -d "Get ManagedRoost resources"
complete -c roost -f -n "__fish_roost_needs_command" -a "create" -d "Create ManagedRoost resources"
complete -c roost -f -n "__fish_roost_needs_command" -a "apply" -d "Apply ManagedRoost configuration"
complete -c roost -f -n "__fish_roost_needs_command" -a "delete" -d "Delete ManagedRoost resources"
complete -c roost -f -n "__fish_roost_needs_command" -a "describe" -d "Describe ManagedRoost resources"
complete -c roost -f -n "__fish_roost_needs_command" -a "status" -d "Show detailed status"
complete -c roost -f -n "__fish_roost_needs_command" -a "logs" -d "Stream logs"
complete -c roost -f -n "__fish_roost_needs_command" -a "health" -d "Health check operations"
complete -c roost -f -n "__fish_roost_needs_command" -a "template" -d "Generate templates"
complete -c roost -f -n "__fish_roost_needs_command" -a "completion" -d "Generate completion scripts"
complete -c roost -f -n "__fish_roost_needs_command" -a "version" -d "Show version information"

# Get command
complete -c roost -f -n "__fish_roost_using_command get" -a "(__fish_roost_get_managedroosts)"
complete -c roost -f -n "__fish_roost_using_command get" -s l -l selector -d "Label selector"
complete -c roost -f -n "__fish_roost_using_command get" -l all-namespaces -d "List from all namespaces"
complete -c roost -f -n "__fish_roost_using_command get" -s w -l watch -d "Watch for changes"

# Create/Apply/Delete commands
complete -c roost -f -n "__fish_roost_using_command create" -s f -l filename -d "YAML file" -F
complete -c roost -f -n "__fish_roost_using_command create" -l dry-run -d "Validate without creating"
complete -c roost -f -n "__fish_roost_using_command apply" -s f -l filename -d "YAML file" -F
complete -c roost -f -n "__fish_roost_using_command apply" -l dry-run -d "Validate without applying"
complete -c roost -f -n "__fish_roost_using_command delete" -s f -l filename -d "YAML file" -F
complete -c roost -f -n "__fish_roost_using_command delete" -l force -d "Force deletion"
complete -c roost -f -n "__fish_roost_using_command delete" -a "(__fish_roost_get_managedroosts)"

# Describe/Status/Logs commands
complete -c roost -f -n "__fish_roost_using_command describe" -a "(__fish_roost_get_managedroosts)"
complete -c roost -f -n "__fish_roost_using_command status" -a "(__fish_roost_get_managedroosts)"
complete -c roost -f -n "__fish_roost_using_command status" -s w -l watch -d "Watch for changes"
complete -c roost -f -n "__fish_roost_using_command logs" -a "(__fish_roost_get_managedroosts)"
complete -c roost -f -n "__fish_roost_using_command logs" -s f -l follow -d "Follow log output"
complete -c roost -f -n "__fish_roost_using_command logs" -l tail -d "Number of lines to show"
complete -c roost -f -n "__fish_roost_using_command logs" -l container -d "Container name"
complete -c roost -f -n "__fish_roost_using_command logs" -l previous -d "Show previous logs"

# Health command
complete -c roost -f -n "__fish_roost_using_command health" -a "check list debug"

# Template command
complete -c roost -f -n "__fish_roost_using_command template" -a "generate validate"
complete -c roost -f -n "__fish_roost_using_command template generate" -l chart -d "Helm chart name"
complete -c roost -f -n "__fish_roost_using_command template generate" -l repo -d "Helm repository URL"
complete -c roost -f -n "__fish_roost_using_command template generate" -l version -d "Chart version"
complete -c roost -f -n "__fish_roost_using_command template generate" -l output -d "Output file" -F
complete -c roost -f -n "__fish_roost_using_command template generate" -l preset -d "Preset template" -a "simple complex monitoring webapp"
complete -c roost -f -n "__fish_roost_using_command template validate" -s f -l filename -d "YAML file" -F

# Completion command
complete -c roost -f -n "__fish_roost_using_command completion" -a "bash zsh fish powershell"
`

	fmt.Print(script)
	return nil
}

// generatePowerShellCompletion generates PowerShell completion script
func generatePowerShellCompletion() error {
	script := `# roost PowerShell completion

Register-ArgumentCompleter -Native -CommandName roost -ScriptBlock {
    param($commandName, $wordToComplete, $cursorPosition)

    $completions = @()
    $command = $wordToComplete.Split(' ')

    switch ($command.Count) {
        1 {
            # Main commands
            $completions += 'get', 'create', 'apply', 'delete', 'describe', 'status', 'logs', 'health', 'template', 'completion', 'version'
        }
        2 {
            switch ($command[0]) {
                'get' {
                    $completions += '--selector', '--all-namespaces', '--watch'
                    # Add ManagedRoost names
                    try {
                        $managedroosts = kubectl get managedroosts -o name 2>$null | ForEach-Object { $_.Split('/')[1] }
                        $completions += $managedroosts
                    } catch {}
                }
                'create' {
                    $completions += '--filename', '--dry-run'
                }
                'apply' {
                    $completions += '--filename', '--dry-run'
                }
                'delete' {
                    $completions += '--filename', '--force'
                    # Add ManagedRoost names
                    try {
                        $managedroosts = kubectl get managedroosts -o name 2>$null | ForEach-Object { $_.Split('/')[1] }
                        $completions += $managedroosts
                    } catch {}
                }
                'describe' {
                    # Add ManagedRoost names
                    try {
                        $managedroosts = kubectl get managedroosts -o name 2>$null | ForEach-Object { $_.Split('/')[1] }
                        $completions += $managedroosts
                    } catch {}
                }
                'status' {
                    $completions += '--watch', '--refresh'
                    # Add ManagedRoost names
                    try {
                        $managedroosts = kubectl get managedroosts -o name 2>$null | ForEach-Object { $_.Split('/')[1] }
                        $completions += $managedroosts
                    } catch {}
                }
                'logs' {
                    $completions += '--follow', '--tail', '--container', '--previous', '--since'
                    # Add ManagedRoost names
                    try {
                        $managedroosts = kubectl get managedroosts -o name 2>$null | ForEach-Object { $_.Split('/')[1] }
                        $completions += $managedroosts
                    } catch {}
                }
                'health' {
                    $completions += 'check', 'list', 'debug'
                }
                'template' {
                    $completions += 'generate', 'validate'
                }
                'completion' {
                    $completions += 'bash', 'zsh', 'fish', 'powershell'
                }
            }
        }
        3 {
            switch ("$($command[0]) $($command[1])") {
                'health check' {
                    $completions += '--check-name', '--wait', '--timeout'
                    # Add ManagedRoost names
                    try {
                        $managedroosts = kubectl get managedroosts -o name 2>$null | ForEach-Object { $_.Split('/')[1] }
                        $completions += $managedroosts
                    } catch {}
                }
                'health list' {
                    # Add ManagedRoost names
                    try {
                        $managedroosts = kubectl get managedroosts -o name 2>$null | ForEach-Object { $_.Split('/')[1] }
                        $completions += $managedroosts
                    } catch {}
                }
                'health debug' {
                    $completions += '--check-name'
                    # Add ManagedRoost names
                    try {
                        $managedroosts = kubectl get managedroosts -o name 2>$null | ForEach-Object { $_.Split('/')[1] }
                        $completions += $managedroosts
                    } catch {}
                }
                'template generate' {
                    $completions += '--chart', '--repo', '--version', '--namespace', '--output', '--preset'
                }
                'template validate' {
                    $completions += '--filename'
                }
            }
        }
    }

    # Global flags
    $completions += '--kubeconfig', '--context', '--namespace', '--output', '--verbose', '--debug', '--help', '--version'

    # Filter completions based on current input
    $filteredCompletions = $completions | Where-Object { $_ -like "$wordToComplete*" }

    foreach ($completion in $filteredCompletions) {
        [System.Management.Automation.CompletionResult]::new($completion, $completion, 'ParameterValue', $completion)
    }
}
`

	fmt.Print(script)
	return nil
}
