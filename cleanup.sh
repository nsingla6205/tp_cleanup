#!/bin/bash

# Navigate to the script directory
cd /Users/nsingla/GolandProjects/tp_cleanup

# Run the cleanup script and log output
/usr/local/go/bin/go run main.go >> /Users/nsingla/GolandProjects/tp_cleanup/cleanup.log 2>&1

# Add timestamp to log
echo "Cleanup completed at $(date)" >> /Users/nsingla/GolandProjects/tp_cleanup/cleanup.log
echo "----------------------------------------" >> /Users/nsingla/GolandProjects/tp_cleanup/cleanup.log
