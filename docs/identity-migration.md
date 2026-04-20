# Identity Migration

Chatbox uses a local identity file to decide which historical messages a user is allowed to recover from other online clients.

## Export Identity

On the old device:

```bash
chatbox identity export --out ./chatbox-identity.json
```

Keep this file private. Anyone with the file can act as the same chat identity for history recovery.

## Import Identity

On the new device:

```bash
chatbox identity import --in ./chatbox-identity.json
```

After import, the new device can recover room history from the original identity's first joined time, as long as another modern client holding that history is online.

## Notes

- The router host still does not store chat history.
- A brand-new identity starts from its first join time and cannot recover older room history.
- If no device with the missing history is online, sync will retry when a holder comes online later.
