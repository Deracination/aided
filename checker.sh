#!/usr/bin/env bash
#
# AIDE Checker - Ansible/Bash reimplementation
# Run system integrity checks using AIDE against remote systems
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DBDIR="${SCRIPT_DIR}/db"
NRETRY=3
VERBOSE=""
UPDATE=""
MAIL=""
WHO="${USER:-$(whoami)}"
SITES=()

# SSH/SCP options for non-interactive use
export ANSIBLE_HOST_KEY_CHECKING=False

usage() {
    cat <<EOF
Usage: $(basename "$0") [OPTIONS] [site names...]

Run AIDE integrity checks on remote systems.

Options:
    --db=DIR          Override database directory (default: ./db)
    --update=NAME     Update AIDE database for specified site
    --mail=ADDRESS    Email results to this address
    --who=USERNAME    SSH username (default: current user)
    --verbose         Enable verbose output
    -h, --help        Show this help message

Examples:
    $(basename "$0")                          # Check all sites
    $(basename "$0") site1 site2              # Check specific sites
    $(basename "$0") --update=mysite          # Update AIDE db for mysite
    $(basename "$0") --mail=admin@example.com # Check all and email results
EOF
    exit 0
}

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

log_verbose() {
    if [[ -n "$VERBOSE" ]]; then
        log "$*"
    fi
}

error() {
    echo "[ERROR] $*" >&2
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --db=*)
                DBDIR="${1#*=}"
                shift
                ;;
            --update=*)
                UPDATE="${1#*=}"
                shift
                ;;
            --mail=*)
                MAIL="${1#*=}"
                shift
                ;;
            --who=*)
                WHO="${1#*=}"
                shift
                ;;
            --verbose)
                VERBOSE="1"
                shift
                ;;
            -h|--help)
                usage
                ;;
            -*)
                error "Unknown option: $1"
                usage
                ;;
            *)
                SITES+=("$1")
                shift
                ;;
        esac
    done
}

# Get list of all sites from db directory
get_all_sites() {
    local sites=()
    if [[ -d "$DBDIR" ]]; then
        for dir in "$DBDIR"/*/; do
            if [[ -d "$dir" ]]; then
                local site
                site=$(basename "$dir")
                # Skip hidden directories
                [[ "$site" != .* ]] && sites+=("$site")
            fi
        done
    fi
    printf '%s\n' "${sites[@]}" | sort
}

# Read site config (check and update commands)
get_site_config() {
    local site="$1"
    local config_file="$DBDIR/$site/config.json"

    # Default commands
    local check_cmd="sudo /usr/sbin/aide --check"
    local update_cmd='["sudo /usr/sbin/aide --update", "echo {SUDO_PW}|sudo -S mv /var/lib/aide/aide.db.new.gz /var/lib/aide/aide.db.gz"]'

    if [[ -r "$config_file" ]]; then
        local custom_check custom_update
        custom_check=$(jq -r '.check // empty' "$config_file" 2>/dev/null)
        custom_update=$(jq -c '.update // empty' "$config_file" 2>/dev/null)

        [[ -n "$custom_check" ]] && check_cmd="$custom_check"
        [[ -n "$custom_update" && "$custom_update" != "null" ]] && update_cmd="$custom_update"
    fi

    echo "$check_cmd"
}

# Generate dynamic Ansible inventory
generate_inventory() {
    local sites=("$@")
    local inventory_file
    inventory_file=$(mktemp)

    echo "[aide_hosts]" > "$inventory_file"
    for site in "${sites[@]}"; do
        local config_file="$DBDIR/$site/config.json"
        local ansible_host="$site"
        local ansible_user="$WHO"

        # Check for custom host/user in config
        if [[ -r "$config_file" ]]; then
            local custom_host custom_user
            custom_host=$(jq -r '.host // empty' "$config_file" 2>/dev/null)
            custom_user=$(jq -r '.user // empty' "$config_file" 2>/dev/null)
            [[ -n "$custom_host" ]] && ansible_host="$custom_host"
            [[ -n "$custom_user" ]] && ansible_user="$custom_user"
        fi

        echo "$site ansible_host=$ansible_host ansible_user=$ansible_user" >> "$inventory_file"
    done

    echo "$inventory_file"
}

# Run AIDE check on a single site
check_site() {
    local site="$1"
    local attempt=1
    local check_cmd
    check_cmd=$(get_site_config "$site")

    echo "################################## Checking $site"

    while [[ $attempt -le $NRETRY ]]; do
        log_verbose "Attempt $attempt/$NRETRY for $site"

        local ssh_opts="-q -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
        [[ -n "$VERBOSE" ]] && ssh_opts="-v -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"

        if ssh $ssh_opts "$site" "$check_cmd"; then
            echo "################################## Finished $site"
            return 0
        else
            if [[ $attempt -lt $NRETRY ]]; then
                local sleep_time=$((RANDOM % 60))
                log_verbose "Check failed, retrying in ${sleep_time}s..."
                sleep "$sleep_time"
            fi
        fi
        ((attempt++))
    done

    echo "################################## Finished $site (FAILED)"
    return 1
}

# Run AIDE check using Ansible playbook
check_sites_ansible() {
    local sites=("$@")
    local inventory_file
    inventory_file=$(generate_inventory "${sites[@]}")

    local verbosity=""
    [[ -n "$VERBOSE" ]] && verbosity="-vv"

    # Run the check playbook
    ansible-playbook \
        -i "$inventory_file" \
        $verbosity \
        "$SCRIPT_DIR/playbooks/check.yml" \
        -e "dbdir=$DBDIR"

    local result=$?
    rm -f "$inventory_file"
    return $result
}

# Run AIDE update for a site
update_site() {
    local site="$1"
    local config_file="$DBDIR/$site/config.json"

    echo "Updating $site"

    # Get update commands
    local update_cmds
    if [[ -r "$config_file" ]]; then
        update_cmds=$(jq -c '.update // null' "$config_file" 2>/dev/null)
    fi

    # Default update commands if not specified
    if [[ -z "$update_cmds" || "$update_cmds" == "null" ]]; then
        update_cmds='["sudo /usr/sbin/aide --update", "echo {SUDO_PW}|sudo -S mv /var/lib/aide/aide.db.new.gz /var/lib/aide/aide.db.gz"]'
    fi

    # Check if we need sudo password
    local needs_sudo_pw=false
    if echo "$update_cmds" | grep -q 'SUDO_PW'; then
        needs_sudo_pw=true
    fi

    local sudo_pw=""
    if [[ "$needs_sudo_pw" == "true" ]]; then
        echo -n "Enter sudo password: "
        read -rs sudo_pw
        echo
    fi

    local ssh_opts="-q -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
    [[ -n "$VERBOSE" ]] && ssh_opts="-v -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"

    # Process each command
    local cmd_count
    cmd_count=$(echo "$update_cmds" | jq 'if type == "array" then length else 1 end')

    for ((i=0; i<cmd_count; i++)); do
        local cmd
        if [[ "$cmd_count" -eq 1 ]] && ! echo "$update_cmds" | jq -e 'type == "array"' >/dev/null 2>&1; then
            cmd=$(echo "$update_cmds" | jq -r '.')
        else
            cmd=$(echo "$update_cmds" | jq -r ".[$i]")
        fi

        # Replace sudo password placeholder
        cmd="${cmd//\{SUDO_PW\}/$sudo_pw}"

        log_verbose "Running: $cmd"

        if ! ssh $ssh_opts "$site" "$cmd"; then
            error "Command failed: $cmd"
            # Continue anyway (matches original behavior with ignore flag)
        fi
    done

    log "Update completed for $site"
}

# Main check function using direct SSH (matching original behavior)
run_checks() {
    local sites=("$@")
    local passed=()
    local failed=()

    for site in "${sites[@]}"; do
        if check_site "$site"; then
            passed+=("$site")
        else
            failed+=("$site")
        fi
    done

    # Generate result message
    local passes_str="NONE"
    local fails_str="NONE"

    [[ ${#passed[@]} -gt 0 ]] && passes_str=$(IFS=', '; echo "${passed[*]}")
    [[ ${#failed[@]} -gt 0 ]] && fails_str=$(IFS=', '; echo "${failed[*]}")

    local msg
    msg=$(cat <<EOF
Passed AIDE checks: $passes_str
Failed: $fails_str

EOF
)

    # Send email if requested
    if [[ -n "$MAIL" ]]; then
        echo "$msg" | mail -s 'AIDE checks' "$MAIL"
    fi

    echo "$msg"
}

# Main entry point
main() {
    parse_args "$@"

    # Validate db directory
    if [[ ! -d "$DBDIR" ]]; then
        error "Database directory not found: $DBDIR"
        exit 1
    fi

    if [[ -n "$UPDATE" ]]; then
        # Update mode
        if [[ -z "$WHO" ]]; then
            error "--who required for update"
            exit 1
        fi
        update_site "$UPDATE"
    else
        # Check mode
        local sites_to_check=()

        if [[ ${#SITES[@]} -gt 0 ]]; then
            # Use specified sites
            sites_to_check=("${SITES[@]}")
        else
            # Get all sites
            mapfile -t sites_to_check < <(get_all_sites)
        fi

        if [[ ${#sites_to_check[@]} -eq 0 ]]; then
            error "No sites found in $DBDIR"
            exit 1
        fi

        run_checks "${sites_to_check[@]}"
    fi
}

main "$@"
