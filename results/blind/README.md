# Blind review

The ten projects were renamed A–J at random before the human "Would I merge it?" review. The letter-to-run key (`map.json`) stays private until all ten are scored. Its SHA-256 fingerprint was published here first, so anyone can check afterwards that the key wasn't changed after the review:

```bash
sha256sum -c map.json.sha256
```

`reviews.json` holds the scores (approve 0–10, understand 0–10, notes) once the review is done.
