{ config, lib, pkgs, ... }:

let
  cfg = config.services.spejder;
  defaultPackage = pkgs.callPackage ./package.nix { };
in
{
  options.services.spejder = {
    enable = lib.mkEnableOption "spejder eBPF network monitor";

    package = lib.mkOption {
      type = lib.types.package;
      default = defaultPackage;
      description = "spejder package to use";
    };

    dbPath = lib.mkOption {
      type = lib.types.str;
      default = "/var/lib/spejder/spejder.db";
      description = "Path to the SQLite database";
    };

    cityDbPath = lib.mkOption {
      type = lib.types.str;
      default = "/var/lib/spejder/geo/GeoLite2-City.mmdb";
      description = "Path to GeoLite2-City.mmdb";
    };

    asnDbPath = lib.mkOption {
      type = lib.types.str;
      default = "/var/lib/spejder/geo/GeoLite2-ASN.mmdb";
      description = "Path to GeoLite2-ASN.mmdb";
    };

    readers = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = [ ];
      example = [ "arved" ];
      description = "Users who may read the database (added to the spejder group)";
    };

    geoip = {
      enable = lib.mkEnableOption "automatic weekly GeoIP database updates";

      configFile = lib.mkOption {
        type = lib.types.str;
        example = "/run/secrets/geoip.conf";
        description = ''
          Path to GeoIP.conf with MaxMind credentials.
          Must be outside the Nix store (file contains secrets).
          See GeoIP.conf.example for the required format.
        '';
      };
    };
  };

  config = lib.mkIf cfg.enable {
    users.groups.spejder = { };

    users.users = lib.mkMerge (map (u: {
      ${u}.extraGroups = [ "spejder" ];
    }) cfg.readers);

    systemd.services.spejder = {
      description = "spejder eBPF network monitor";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];

      serviceConfig = {
        ExecStartPre = lib.mkIf cfg.geoip.enable (pkgs.writeShellScript "spejder-init-geo" ''
          mkdir -p /var/lib/spejder/geo
          if [ ! -f ${lib.escapeShellArg cfg.cityDbPath} ]; then
            echo "GeoIP databases missing, downloading..."
            ${pkgs.geoipupdate}/bin/geoipupdate -f ${lib.escapeShellArg cfg.geoip.configFile} -d /var/lib/spejder/geo
          fi
        '');
        ExecStart = lib.escapeShellArgs [
          "${cfg.package}/bin/spejder-daemon"
          "-db" cfg.dbPath
          "-city-db" cfg.cityDbPath
          "-asn-db" cfg.asnDbPath
        ];
        Restart = "on-failure";
        RestartSec = "5s";
        User = "root";
        Group = "spejder";
        StateDirectory = "spejder";
        StateDirectoryMode = "0750";
        LogsDirectory = "spejder";
      };
    };

    systemd.services.spejder-geoip-update = lib.mkIf cfg.geoip.enable {
      description = "Update GeoIP databases for spejder";

      serviceConfig = {
        Type = "oneshot";
        ExecStartPre = "${pkgs.coreutils}/bin/mkdir -p /var/lib/spejder/geo";
        ExecStart = "${pkgs.geoipupdate}/bin/geoipupdate -f ${cfg.geoip.configFile} -d /var/lib/spejder/geo";
        User = "root";
      };
    };

    systemd.timers.spejder-geoip-update = lib.mkIf cfg.geoip.enable {
      description = "Weekly GeoIP database update for spejder";
      wantedBy = [ "timers.target" ];

      timerConfig = {
        OnCalendar = "weekly";
        OnBootSec = "2min"; # also run shortly after boot if databases are missing/stale
        Persistent = true;  # catch up if the server was off during the scheduled time
      };
    };
  };
}
