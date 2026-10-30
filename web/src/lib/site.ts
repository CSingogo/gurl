export const site = {
  name: "gurl",
  tagline: "Gurl is curl, but written in Go.",
  description:
    "gurl is a curl-compatible HTTP and WebSocket client written in Go, with readable output in your terminal and byte-exact output in your pipes.",
  url: "https://gurl.somosingogo.dev",
  repo: "https://github.com/CSingogo/gurl",
  releases: "https://github.com/CSingogo/gurl/releases",
  // Points at the GitHub Sponsors page; swap it if Sponsors isn't available in your region.
  sponsor: "https://github.com/sponsors/CSingogo",
  author: "https://somosingogo.dev",
  installScript: "https://gurl.somosingogo.dev/install.sh",
  goInstall: "go install github.com/CSingogo/gurl/cmd/gurl@latest",
} as const;

export type OsId = "macos" | "ubuntu" | "arch" | "linux" | "windows";

export const osList: { id: OsId; label: string }[] = [
  { id: "macos", label: "macOS" },
  { id: "ubuntu", label: "Ubuntu / Debian" },
  { id: "arch", label: "Arch Linux" },
  { id: "linux", label: "Other Linux" },
  { id: "windows", label: "Windows" },
];
