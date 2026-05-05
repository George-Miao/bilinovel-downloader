{
  inputs = {
    nixpkgs.url = "nixpkgs";
  };

  outputs =
    { self, nixpkgs }:
    let
      system = "x86_64-linux";
      pkgs = nixpkgs.legacyPackages."${system}";
      inherit (pkgs) lib;
      version = self.shortRev or self.dirtyShortRev or "dev";

      chromiumExecutable =
        let
          browsers = pkgs.playwright-driver.browsers;
          dirs = builtins.readDir browsers;
          chromiumDir = lib.findFirst
            (name: lib.hasPrefix "chromium_headless_shell-" name)
            (throw "No chromium_headless_shell directory found in playwright browsers")
            (builtins.attrNames dirs);
        in
        "${browsers}/${chromiumDir}/chrome-headless-shell-linux64/chrome-headless-shell";
    in
    {
      packages.${system} =
        let
          bilinovel-downloader = pkgs.buildGoModule {
            pname = "bilinovel-downloader";
            inherit version;

            src = lib.cleanSource self;
            vendorHash = "sha256-M/59RtblC4u11IDf9bkA8ZQfa5QT9up3n4d9xNQCkIY=";

            ldflags = [
              "-s"
              "-w"
              "-X"
              "bilinovel-downloader/cmd.Version=${version}"
            ];
          };

          server = pkgs.writeShellApplication {
            name = "bilinovel-downloader-server";
            runtimeInputs = [
              bilinovel-downloader
            ];
            text = ''
              export PLAYWRIGHT_MCP_EXECUTABLE_PATH="${chromiumExecutable}"

              exec bilinovel-downloader server
            '';
          };
        in
        {
          default = bilinovel-downloader;
          inherit bilinovel-downloader server;

          docker = pkgs.dockerTools.buildLayeredImage {
            name = "bilinovel-downloader";
            tag = version;
            contents = [
              server
              pkgs.cacert
              pkgs.playwright-driver.browsers
            ];
            config = {
              Cmd = [ "${server}/bin/bilinovel-downloader-server" ];
              Env = [
                "DOWNLOAD_DIR=/downloads"
                "AUX_DIR=/aux"
                "CLEAN_AUX_FILES=false"
                "SERVER_ADDR=:8080"
              ];
              ExposedPorts = {
                "8080/tcp" = { };
              };
              Volumes = {
                "/downloads" = { };
                "/aux" = { };
              };
            };
          };
        };

      devShells.${system}.default =
        with pkgs;
        mkShell {
          packages = [
            go
          ];

          nativeBuildInputs = [
            playwright-driver.browsers
          ];

          shellHook = ''
            export PLAYWRIGHT_MCP_EXECUTABLE_PATH=${chromiumExecutable}
          '';
        };
    };
}
