// Guards the jury Reset button: nuclear wipe of all competition data and files.
// A misclick would be catastrophic, so require an explicit confirm plus typing
// the exact phrase. Returns false to cancel the form submit.
// ponytail: static phrase "RESET"; upgrade to the competition name if the app
// ever manages more than one competition.
function confirmReset() {
  if (!confirm("This deletes ALL competition data, participants, files, and submissions. This cannot be undone. Continue?")) {
    return false;
  }
  return prompt('Type RESET to confirm:') === "RESET";
}
