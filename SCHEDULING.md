# Scheduling GCP Cleanup Script

This guide explains how to schedule the cleanup script to run daily at 4:00 AM IST on macOS.

## Setup Instructions

### 1. Make the shell script executable

```bash
chmod +x /Users/nsingla/GolandProjects/tp_cleanup/cleanup.sh
```

### 2. Copy the Launch Agent to the proper location

```bash
cp /Users/nsingla/GolandProjects/tp_cleanup/com.gcp.cleanup.plist ~/Library/LaunchAgents/
```

### 3. Load the Launch Agent

```bash
launchctl load ~/Library/LaunchAgents/com.gcp.cleanup.plist
```

### 4. Verify it's loaded

```bash
launchctl list | grep com.gcp.cleanup
```

## Important Notes

### Time Zone Consideration

⚠️ **The plist file is configured for 4:00 AM in your system's local time zone.**

If your Mac is set to IST (Indian Standard Time), it will run at 4:00 AM IST.
To verify your time zone:
```bash
date
```

### Testing the Schedule

To test if the script runs correctly:
```bash
launchctl start com.gcp.cleanup
```

Check the logs:
```bash
cat /Users/nsingla/GolandProjects/tp_cleanup/cleanup.log
cat /Users/nsingla/GolandProjects/tp_cleanup/cleanup_error.log
```

## Managing the Scheduled Task

### Stop the scheduled task
```bash
launchctl unload ~/Library/LaunchAgents/com.gcp.cleanup.plist
```

### Restart the scheduled task
```bash
launchctl unload ~/Library/LaunchAgents/com.gcp.cleanup.plist
launchctl load ~/Library/LaunchAgents/com.gcp.cleanup.plist
```

### Remove the scheduled task completely
```bash
launchctl unload ~/Library/LaunchAgents/com.gcp.cleanup.plist
rm ~/Library/LaunchAgents/com.gcp.cleanup.plist
```

## Log Files

- **cleanup.log**: Contains output from successful runs
- **cleanup_error.log**: Contains error messages if any

View recent logs:
```bash
tail -f /Users/nsingla/GolandProjects/tp_cleanup/cleanup.log
```

## Alternative: Using Cron (Not Recommended on macOS)

If you prefer cron (though launchd is preferred on macOS):

1. Edit crontab:
```bash
crontab -e
```

2. Add this line for 4:00 AM IST daily:
```
0 4 * * * /bin/bash /Users/nsingla/GolandProjects/tp_cleanup/cleanup.sh
```

3. Save and exit.

Note: macOS may require giving cron Full Disk Access in System Preferences > Security & Privacy > Privacy > Full Disk Access.

## Troubleshooting

### Check if the job is scheduled
```bash
launchctl list | grep com.gcp.cleanup
```

### View system logs for launchd
```bash
log show --predicate 'process == "launchd"' --last 1h | grep cleanup
```

### Make sure GCP credentials are available
The script needs GCP authentication. Ensure:
```bash
gcloud auth application-default login
```

### Verify Go is in the PATH
The script uses the full path to Go: `/usr/local/go/bin/go`
If your Go is installed elsewhere, update `cleanup.sh` with the correct path:
```bash
which go
```
