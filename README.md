# GCP Resource Cleanup Script

This Go script cleans up GCP resources across multiple projects, including:
- VM instances
- Unattached disks
- Unused static IP addresses (regional and global)
- Storage buckets (with all objects)

## Prerequisites

1. **Go 1.25+** installed
2. **GCP Authentication**: You need to be authenticated with GCP. Run:
   ```bash
   gcloud auth application-default login
   ```

3. **Required IAM Permissions**: Your account needs the following permissions:
   - `compute.instances.delete`
   - `compute.disks.delete`
   - `compute.addresses.delete`
   - `storage.buckets.delete`
   - `storage.objects.delete`

## Configuration

Edit `main.go` and configure:

1. **Project IDs** - Add your GCP project IDs:
   ```go
   projectIDs = []string{
       "your-project-id-1",
       "your-project-id-2",
   }
   ```

2. **Dry Run Mode** - Set to `false` to actually delete resources:
   ```go
   dryRun = true  // Set to false to perform actual deletions
   ```

3. **Optional Filters**:
   - `zones`: Limit to specific zones (empty = all zones)
   - `regions`: Limit to specific regions for IPs (empty = all regions)

## Usage

1. **Install dependencies**:
   ```bash
   go mod download
   ```

2. **Run in dry-run mode** (recommended first):
   ```bash
   go run main.go
   ```
   This will list all resources that would be deleted without actually deleting them.

3. **Run actual cleanup** (after verifying dry-run output):
   - Set `dryRun = false` in `main.go`
   - Run:
     ```bash
     go run main.go
     ```

## Safety Features

- **Dry-run by default**: Won't delete anything unless you set `dryRun = false`
- **Skip in-use resources**: 
  - Skips disks attached to VM instances
  - Skips static IPs currently in use
- **Concurrent processing**: Processes different resource types in parallel for efficiency
- **Detailed logging**: Shows what's being checked and deleted

## Build

To create a standalone binary:
```bash
go build -o gcp-cleanup
./gcp-cleanup
```

## Example Output

```
Starting GCP cleanup script (Dry Run: true)
Projects to clean: [project-1 project-2]

========== Processing Project: project-1 ==========
[project-1] Checking VM instances...
  Found VM Instance: test-vm (zone: us-central1-a, status: RUNNING)
[project-1] Would delete 1 VM instances

[project-1] Checking disks...
  Found Disk: test-disk (zone: us-central1-a, size: 100 GB)
[project-1] Would delete 1 disks

[project-1] Checking static IP addresses...
  Found Static IP: test-ip (region: us-central1, address: 35.1.2.3, status: RESERVED)
[project-1] Would release 1 static IPs

[project-1] Checking storage buckets...
  Found Bucket: test-bucket (location: US, storage class: STANDARD)
[project-1] Would delete 1 buckets

========== Cleanup Complete ==========
```

## Warning

⚠️ **This script will permanently delete resources. Use with caution!**

Always run in dry-run mode first and verify the output before setting `dryRun = false`.
