// Package sync mirrors a romctl library to a device over rsync/SSH,
// wrapping the system rsync and ssh binaries (not reimplemented in Go:
// replicating the rsync protocol is out of scope).
package sync

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/demicloud/romctl/internal/config"
	"github.com/demicloud/romctl/internal/executil"
	"github.com/demicloud/romctl/internal/retrodeck"
)

// defaultSyncUser is the standard Steam Deck / RetroDECK username.
const defaultSyncUser = "deck"

// excludeCommon lists paths never transferred, and (because rsync does not
// delete excluded files on the receiver) never deleted there either:
// save/state files, conversion leftovers, pending deletions.
var excludeCommon = []string{
	"*.sav", "*.srm", "*.state", "*.state[0-9]*", "*.auto",
	"*.partial.*", "*.tmp.*", "*.romctl-rename.*", "_to_delete/", "_extract.*/",
}

// excludeBios lists emulator-written data under bios/ that every mirror
// must leave untouched on the device: saves, NVRAM and caches.
var excludeBios = []string{
	"xbox_hdd.qcow2", "*.nvm", "*.mec", "*.mcd", "*.mcr",
	"dc/vmu_save_*", "dc/dc_nvmem.bin", "dc/dc_flash.bin", "dc/vulkan_pipeline.cache",
	"pcsx2/memcards/", "pcsx2/logs/", "cemu/usr/save/", "switch/registered/",
	"Mupen64plus/cache/", "Mupen64plus/shaders/", "swanstation/",
}

// targetDirs maps a sync target name to its directory name under the
// library root / device path. "media" covers ES-DE's scraped metadata
// (gamelist.xml and downloaded_media), which RetroDECK stores as a
// sibling of roms/ and bios/ under the same root.
var targetDirs = map[string]string{
	"roms":  "roms",
	"bios":  "bios",
	"media": "ES-DE",
}

// Options configures a sync run.
type Options struct {
	Root    string
	DryRun  bool
	Pull    bool // reverse direction: device -> local, never deletes locally
	Config  config.Config
	Targets []string // "roms", "bios", "media"; defaults depend on Pull
}

// Run mirrors each requested target between the library root and the
// configured device.
func Run(opts Options) error {
	targets := opts.Targets
	if len(targets) == 0 {
		if opts.Pull {
			// A pull with no target named is "populate everything I don't
			// have locally yet" (initial setup, or grabbing media for the
			// first time); a push defaults to roms+bios only, since
			// pushing media is a deliberate, explicit action (see README).
			targets = []string{"roms", "bios", "media"}
		} else {
			targets = []string{"roms", "bios"}
		}
	}
	for _, t := range targets {
		dir, ok := targetDirs[t]
		if !ok {
			return fmt.Errorf("sync target must be roms, bios or media: %s", t)
		}
		if opts.Pull {
			continue // local dir need not exist yet; rsync creates it
		}
		if info, err := os.Stat(opts.Root + "/" + dir); err != nil || !info.IsDir() {
			return fmt.Errorf("no %s/ directory under %s", dir, opts.Root)
		}
	}
	if opts.Config.Sync.Host == "" {
		return fmt.Errorf("set [sync] host in the config file (see --help)")
	}
	user := opts.Config.Sync.User
	if user == "" {
		user = defaultSyncUser
	}
	for _, tool := range []string{"rsync", "ssh"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("required command not found: %s", tool)
		}
	}

	// A multiplexed connection means a password (if the user has no SSH
	// key set up) is only ever asked for once per sync run, no matter how
	// many ssh/rsync-over-ssh calls follow - the retrodeck.json lookup
	// plus one rsync invocation per target. Best-effort: if it can't be
	// established, every call below just falls back to its own prompt.
	mux := newSSHMux(user, opts.Config.Sync.Host)
	defer mux.Close()

	devicePath := opts.Config.Sync.Path
	if devicePath == "" {
		resolved, err := resolveRetroDeckHome(user, opts.Config.Sync.Host, mux)
		if err != nil {
			return err
		}
		devicePath = resolved
	}

	for _, t := range targets {
		dir := targetDirs[t]
		args := []string{"-rlt", "--human-readable"}
		if rsh := mux.rsh(); rsh != "" {
			args = append(args, "-e", rsh)
		}
		if !opts.Pull {
			args = append(args, "--delete")
		}
		if opts.DryRun {
			args = append(args, "--dry-run", "--itemize-changes")
		} else {
			args = append(args, "--info=progress2,del,name0", "--partial")
		}
		for _, pattern := range excludeCommon {
			args = append(args, "--exclude="+pattern)
		}
		for _, pattern := range opts.Config.Sync.Exclude {
			args = append(args, "--exclude="+pattern)
		}
		if t == "bios" {
			for _, pattern := range excludeBios {
				args = append(args, "--exclude="+pattern)
			}
		}
		local := opts.Root + "/" + dir + "/"
		remote := fmt.Sprintf("%s@%s:%s/%s/", user, opts.Config.Sync.Host, devicePath, dir)
		src, dst := local, remote
		if opts.Pull {
			src, dst = remote, local
		}
		args = append(args, "--", src, dst)

		if opts.DryRun {
			fmt.Printf("plan: sync %s/ -> %s (dry run; lines starting with *deleting are removals)\n", t, dst)
		} else {
			fmt.Printf("do:   sync %s/ -> %s\n", t, dst)
		}
		if err := executil.RunInherit("rsync", args...); err != nil {
			return fmt.Errorf("rsync failed for %s/: %w", t, err)
		}
	}
	return nil
}

// resolveRetroDeckHome reads RetroDECK's own config over a single SSH call
// and returns rd_home_path, the library root - correct whether the install
// lives on internal storage or an SD card, without romctl needing to know
// which. Without a multiplexed connection (mux), this needs key-based SSH
// auth: it's non-interactive and can't prompt for a password.
func resolveRetroDeckHome(user, host string, mux *sshMux) (string, error) {
	target := fmt.Sprintf("%s@%s", user, host)
	args := append(mux.sshArgs(), target, "cat", retrodeck.RemotePath)
	out, err := executil.Run("ssh", args...)
	if err != nil {
		return "", fmt.Errorf("reading RetroDECK config on %s: %w: %s (set [sync] path to override)", host, err, out)
	}
	rdHome, err := retrodeck.ParseRDHomePath(out)
	if err != nil {
		return "", fmt.Errorf("parsing RetroDECK config from %s: %w (set [sync] path to override)", host, err)
	}
	return rdHome, nil
}

// sshMux is a persistent, multiplexed SSH connection (OpenSSH
// ControlMaster/ControlPath) to one host, authenticated once so every
// ssh/rsync-over-ssh call made through it for the rest of a sync run
// reuses the same connection instead of prompting again. The zero value
// is inactive: every method on it is then a harmless no-op, so a sync run
// still works, just with one prompt per connection, if the master can't
// be established (e.g. ControlMaster isn't supported in this
// environment).
type sshMux struct {
	dir, path, target string
	active            bool
}

// newSSHMux starts a multiplexed connection to user@host. Any password or
// host-key prompt happens here, in the foreground, once.
func newSSHMux(user, host string) *sshMux {
	target := fmt.Sprintf("%s@%s", user, host)
	dir, err := os.MkdirTemp("", "romctl-ssh")
	if err != nil {
		return &sshMux{}
	}
	path := filepath.Join(dir, "control")
	if err := executil.RunInherit("ssh",
		"-o", "ControlMaster=yes", "-o", "ControlPath="+path,
		"-N", "-f", target); err != nil {
		os.RemoveAll(dir)
		return &sshMux{}
	}
	return &sshMux{dir: dir, path: path, target: target, active: true}
}

// sshArgs returns the ssh options needed to join the multiplexed
// connection, or nil if none is active.
func (m *sshMux) sshArgs() []string {
	if m == nil || !m.active {
		return nil
	}
	return []string{"-o", "ControlPath=" + m.path}
}

// rsh returns the value for rsync's --rsh/-e flag to route its SSH
// transport through the multiplexed connection, or "" to leave rsync's
// default ssh invocation untouched.
func (m *sshMux) rsh() string {
	if m == nil || !m.active {
		return ""
	}
	return "ssh -o ControlPath=" + m.path
}

// Close asks the master connection to exit and cleans up its control
// socket directory. A no-op on an inactive mux.
func (m *sshMux) Close() {
	if m == nil || !m.active {
		return
	}
	_, _ = executil.Run("ssh", "-o", "ControlPath="+m.path, "-O", "exit", m.target)
	os.RemoveAll(m.dir)
}
