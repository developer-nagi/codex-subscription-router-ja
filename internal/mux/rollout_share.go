package mux

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// shareRolloutWithAccount makes a chat's history visible to another subscription.
//
// The app-server finds a chat by its id inside its own Codex home. A chat recorded in
// one account's home is therefore invisible to another account's app-server no matter
// what path the resume request carries, and the resume fails with "no rollout found for
// thread id". Giving the target home a directory entry for the same file solves that.
//
// A hard link is used rather than a copy: a history runs to hundreds of megabytes on a
// long chat, and duplicating it during a handover is exactly when that cost is least
// affordable. Both names refer to one file, so the turn the target appends stays visible
// to the account the chat came from. A copy is the fallback for the cases a link cannot
// cover, such as the two homes living on different volumes.
func (m *Multiplexer) shareRolloutWithAccount(
	rolloutPath, sourceAccountID, targetAccountID string,
) (string, error) {
	source, ok := m.store.Account(sourceAccountID)
	if !ok {
		return "", fmt.Errorf("subscription %s is not configured", sourceAccountID)
	}
	target, ok := m.store.Account(targetAccountID)
	if !ok {
		return "", fmt.Errorf("subscription %s is not configured", targetAccountID)
	}
	relative, err := relativeToHome(source.CodexHome, rolloutPath)
	if err != nil {
		return "", err
	}
	targetPath := filepath.Join(target.CodexHome, relative)
	if _, err := os.Stat(targetPath); err == nil {
		return targetPath, nil
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", fmt.Errorf("prepare history directory: %w", err)
	}
	if err := os.Link(rolloutPath, targetPath); err == nil {
		trace.note(targetAccountID, "rollout-linked", targetPath)
		return targetPath, nil
	}
	if err := copyFile(rolloutPath, targetPath); err != nil {
		return "", fmt.Errorf("share chat history: %w", err)
	}
	trace.note(targetAccountID, "rollout-copied", targetPath)
	return targetPath, nil
}

// relativeToHome keeps the history in the same place under the target home, which is how
// the app-server expects to find it. A path outside the source home is refused rather
// than written to a guessed location.
func relativeToHome(home, path string) (string, error) {
	cleanHome := filepath.Clean(normalizeExtendedPath(home))
	cleanPath := filepath.Clean(normalizeExtendedPath(path))
	relative, err := filepath.Rel(cleanHome, cleanPath)
	if err == nil && relative != "." && !strings.HasPrefix(relative, "..") &&
		!filepath.IsAbs(relative) {
		return relative, nil
	}
	// Windows reports the same directory in different cases depending on which component
	// produced the path, and Rel reads those as different places.
	prefix := cleanHome + string(filepath.Separator)
	if len(cleanPath) > len(prefix) && strings.EqualFold(cleanPath[:len(prefix)], prefix) {
		return cleanPath[len(prefix):], nil
	}
	return "", fmt.Errorf("chat history is not stored under its subscription")
}

// normalizeExtendedPath removes the Windows extended-length prefix.
//
// The app-server reports a chat's history as \\?\C:\Users\... That names the same file as
// the plain form, but the two forms cannot be compared, so a history was refused as if it
// lived outside its own subscription. Only the comparison uses the plain form: the
// original path is what gets opened, so a path long enough to need the prefix still works.
func normalizeExtendedPath(value string) string {
	const uncPrefix = `\\?\UNC\`
	const devicePrefix = `\\?\`
	if strings.HasPrefix(value, uncPrefix) {
		return `\\` + value[len(uncPrefix):]
	}
	if strings.HasPrefix(value, devicePrefix) {
		return value[len(devicePrefix):]
	}
	return value
}

func copyFile(sourcePath, targetPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	// A partial file would look like a corrupted history, so the copy is completed under
	// a temporary name and only then given the name the app-server looks for.
	temporary := targetPath + ".codex-mux-partial"
	target, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(target, source); err != nil {
		target.Close()
		os.Remove(temporary)
		return err
	}
	if err := target.Close(); err != nil {
		os.Remove(temporary)
		return err
	}
	if err := os.Rename(temporary, targetPath); err != nil {
		os.Remove(temporary)
		return err
	}
	return nil
}
