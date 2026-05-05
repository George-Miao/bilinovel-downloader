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
      browsers = pkgs.playwright-driver.browsers;
      chromiumLibPath = lib.makeLibraryPath pkgs.playwright-driver.components.chromium-headless-shell.buildInputs;

      chromiumExecutable =
        let
          dirs = builtins.readDir browsers;
          chromiumDir =
            lib.findFirst (name: lib.hasPrefix "chromium_headless_shell-" name)
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
          bilinovel-downloader-docker = pkgs.dockerTools.buildLayeredImage {
            name = "bilinovel-downloader";
            tag = version;
            extraCommands = ''
              mkdir -p tmp
              chmod 1777 tmp
            '';
            contents = [
              pkgs.cacert
              pkgs.nodejs
              browsers
            ];
            config = {
              Cmd = [
                "${bilinovel-downloader}/bin/bilinovel-downloader"
                "server"
              ];
              Env = [
                "DOWNLOAD_DIR=/downloads"
                "SERVER_ADDR=:8080"
                "LD_LIBRARY_PATH=${chromiumLibPath}"
                "PLAYWRIGHT_EXECUTABLE_PATH=${chromiumExecutable}"
                "PLAYWRIGHT_NODEJS_PATH=${pkgs.nodejs}/bin/node"
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
        in
        {
          default = bilinovel-downloader;
          inherit bilinovel-downloader bilinovel-downloader-docker;
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
            export LD_LIBRARY_PATH=${chromiumLibPath}''${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}
            export PLAYWRIGHT_EXECUTABLE_PATH=${chromiumExecutable}
          '';
        };
    };
}
