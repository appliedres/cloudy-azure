# ── Stage 1 : AD Domain Join ────────────────────────────────────────────────

# replaced via go
AD_DOMAIN="$AD_DOMAIN"
AD_USER="$AD_USER"
AD_PASS="$AD_PASS"
AD_OU="$AD_OU"
AD_DC="$AD_DC"

# generated
AD_REALM=$(echo "$AD_DOMAIN" | tr '[:lower:]' '[:upper:]')
VM_NAME=$(hostname -s | tr '[:upper:]' '[:lower:]')

install_ad_packages() {
  log "Installing packages for AD join..."
  if [ -f /etc/os-release ]; then
    . /etc/os-release
  else
    log "[ERROR] Cannot find /etc/os-release; aborting."
    exit 1
  fi

  case "$ID" in
    debian|ubuntu)
      export DEBIAN_FRONTEND=noninteractive
      apt-get update -y
      apt-get install -y realmd sssd sssd-tools adcli oddjob oddjob-mkhomedir \
        samba-common-bin packagekit dnsutils expect krb5-user
      ;;
    rhel|centos|rocky|almalinux|fedora)
      yum install -y realmd sssd sssd-tools adcli oddjob oddjob-mkhomedir \
        samba-common-tools expect krb5-workstation
      ;;
    *)
      log "[ERROR] Unsupported distribution: $ID"
      exit 1
      ;;
  esac
}

configure_dns() {
  # ------------------------------------------------------------------------- #
  # Best‑effort DNS setup – prefers systemd‑resolved (resolvectl)             #
  # ------------------------------------------------------------------------- #
  # • Validates / discovers $AD_DC and $AD_DOMAIN                             #
  # • Uses resolvectl when available & active                                 #
  # • Falls back to writing /etc/resolv.conf                                  #
  # • Never exits the caller; only logs WARN on failure                       #
  # ------------------------------------------------------------------------- #
  log "Configuring DNS (prefer resolvectl) …"

  # pick the first default‑route interface
  IFACE=$(ip route | awk '/^default/ {print $5; exit}')
  if [[ -z $IFACE ]]; then
    log "[WARN] No default route found – skipping DNS setup"
    return
  fi

  # ----- ensure we have a DNS server ---------------------------------------
  if [[ -z ${AD_DC:-} ]]; then
    log "[WARN] AD_DC is empty – trying SRV discovery for ${AD_DOMAIN:-<unset>}"
    if command -v dig &>/dev/null && [[ -n ${AD_DOMAIN:-} ]]; then
      AD_DC=$(dig +short _ldap._tcp."$AD_DOMAIN" SRV | awk '{print $4; exit}')
      [[ -n $AD_DC ]] && log "Discovered DC via SRV: $AD_DC"
    fi
  fi

  # ----- try resolvectl -----------------------------------------------------
  if command -v resolvectl &>/dev/null && systemctl is-active --quiet systemd-resolved; then
    [[ -n $AD_DC ]]     && resolvectl dns    "$IFACE" "$AD_DC"     \
                           && log "resolvectl: $IFACE → DNS $AD_DC" \
                           || log "[WARN] resolvectl dns failed (AD_DC='$AD_DC')"
    [[ -n $AD_DOMAIN ]] && resolvectl domain "$IFACE" "$AD_DOMAIN" \
                           && log "resolvectl: $IFACE → search $AD_DOMAIN" \
                           || log "[WARN] resolvectl domain failed (AD_DOMAIN='$AD_DOMAIN')"

    # If at least one of the resolvectl calls succeeded we're done
    [[ -n $AD_DC || -n $AD_DOMAIN ]] && return
  else
    log "[WARN] resolvectl unavailable or systemd‑resolved inactive – falling back to /etc/resolv.conf"
  fi

  # ----- final fallback -----------------------------------------------------
  {
    [[ -n $AD_DC     ]] && printf 'nameserver %s\n' "$AD_DC"
    [[ -n $AD_DOMAIN ]] && printf 'search %s\n'     "$AD_DOMAIN"
  } | sudo tee /etc/resolv.conf >/dev/null

  log "Updated /etc/resolv.conf directly (may be overwritten by NetworkManager)"
}

join_ad_domain() {
  log "Checking if already joined to $AD_DOMAIN..."
  if realm list | grep -q "^$AD_DOMAIN$"; then
    log "Already joined to $AD_DOMAIN. Skipping join."
    return
  fi

  log "Joining domain $AD_DOMAIN with computer name $VM_NAME"
  expect <<EOF
spawn realm join -v -U "$AD_USER" --computer-name=$VM_NAME ${AD_OU:+--computer-ou=$AD_OU} "$AD_DOMAIN"
expect "Password for *:"
send "$AD_PASS\r"
expect eof
EOF

  log "Validating that the system is now joined to $AD_DOMAIN..."
  if realm list | grep -q "^$AD_DOMAIN$"; then
    log "Successfully joined to $AD_DOMAIN"
  else
    log "[ERROR] Domain join appears to have failed — $AD_DOMAIN not listed in realm list"
    exit 1
  fi
}

patch_sssd_conf() {
  local SSSD_CONF="/etc/sssd/sssd.conf"
  log "Patching $SSSD_CONF"

  if [ ! -f "$SSSD_CONF" ]; then
    mkdir -p /etc/sssd
    touch "$SSSD_CONF"
    echo -e "[sssd]\ndomains = $AD_DOMAIN\nconfig_file_version = 2\nservices = nss, pam\n\n[domain/$AD_DOMAIN]" > "$SSSD_CONF"
  fi

  chmod 600 "$SSSD_CONF"

  # Clean conflicting settings
  sed -i '/^access_provider/d' "$SSSD_CONF"
  sed -i '/^ad_access_filter/d' "$SSSD_CONF"
  sed -i '/^simple_allow_users/d' "$SSSD_CONF"
  sed -i '/^use_fully_qualified_names/d' "$SSSD_CONF"
  sed -i '/^fallback_homedir/d' "$SSSD_CONF"
  sed -i '/^default_shell/d' "$SSSD_CONF"

  # Insert enforced access configuration
  sed -i "/^\[domain\/.*\]/a access_provider = permit" "$SSSD_CONF"

  # Insert default options if not already present
  echo "use_fully_qualified_names = false" >> "$SSSD_CONF"
  echo "fallback_homedir = /home/%u" >> "$SSSD_CONF"
  echo "default_shell = /bin/bash" >> "$SSSD_CONF"

  systemctl restart sssd
  log "$SSSD_CONF patched with access_provider=permit (unrestricted AD logins)"
}

verify_ad_status() {
  log "Verifying AD domain join status..."

  # Check if realm list includes the domain
  if realm list | grep -q "$AD_DOMAIN"; then
    log "realm list includes $AD_DOMAIN"
  else
    log "[ERROR] realm list does not include $AD_DOMAIN"
    exit 1
  fi

  # Check sssctl domain status is online
  if sssctl domain-status "$AD_DOMAIN" | grep -q "Online status: Online"; then
    log "SSSD reports domain $AD_DOMAIN is Online"
  else
    log "[ERROR] SSSD domain status is not Online for $AD_DOMAIN"
    sssctl domain-status "$AD_DOMAIN"
    exit 1
  fi

  # Check user can be resolved without FQDN suffix
  TEST_USER="${AD_USER,,}"
  if getent passwd "$TEST_USER" > /dev/null; then
    log "User $TEST_USER is resolvable without domain suffix"
  else
    log "[ERROR] getent could not resolve user $TEST_USER (without domain suffix)"
    log "Attempting getent with FQDN: $TEST_USER@$AD_DOMAIN"
    getent passwd "$TEST_USER@$AD_DOMAIN" || true
    exit 1
  fi
}

# ── Execute AD join sequence ────────────────────────────────────────────────
install_ad_packages
configure_dns
join_ad_domain
patch_sssd_conf
sleep 5  # time to stabilize
verify_ad_status
