package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"cloud.google.com/go/storage"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/iterator"
)

// Configuration
var (
	// List your GCP project IDs here
	projectIDs = []string{
		"z257c6412e15fa257-tp",
		"mfc4dc04826a8d270-tp",
		"u6c7e2e4892fda638-tp",
	}

	// Set to true to actually delete resources, false for dry-run
	dryRun = false
)

func main() {
	ctx := context.Background()

	log.Printf("Starting GCP cleanup script (Dry Run: %v)", dryRun)
	log.Printf("Projects to clean: %v", projectIDs)

	for _, projectID := range projectIDs {
		log.Printf("\n========== Processing Project: %s ==========", projectID)
		cleanupProject(ctx, projectID)
	}

	log.Println("\n========== Cleanup Complete ==========")
}

func cleanupProject(ctx context.Context, projectID string) {
	// Delete VMs first and wait for completion
	if err := deleteVMInstances(ctx, projectID); err != nil {
		log.Printf("Error deleting VM instances in %s: %v", projectID, err)
	}

	// After VMs are deleted, run other cleanup tasks concurrently (skip buckets)
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		if err := deleteDisks(ctx, projectID); err != nil {
			log.Printf("Error deleting disks in %s: %v", projectID, err)
		}
	}()

	go func() {
		defer wg.Done()
		if err := releaseStaticIPs(ctx, projectID); err != nil {
			log.Printf("Error releasing static IPs in %s: %v", projectID, err)
		}
	}()

	go func() {
		defer wg.Done()
		if err := deleteServiceAccounts(ctx, projectID); err != nil {
			log.Printf("Error deleting service accounts in %s: %v", projectID, err)
		}
	}()

	wg.Wait()

	// Note: Buckets require manual deletion confirmation via console
	fmt.Printf("\033[31m[%s] Note: Storage buckets must be deleted manually via GCP Console\033[0m\n", projectID)
}

func deleteVMInstances(ctx context.Context, projectID string) error {
	log.Printf("[%s] Checking VM instances...", projectID)

	client, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create instances client: %w", err)
	}
	defer client.Close()

	// Use aggregated list to get all instances across all zones
	req := &computepb.AggregatedListInstancesRequest{
		Project: projectID,
	}

	// Collect all instances first
	type instanceInfo struct {
		name string
		zone string
	}
	var instances []instanceInfo

	it := client.AggregatedList(ctx, req)
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("error listing instances: %w", err)
		}

		for _, instance := range pair.Value.Instances {
			zone := extractZoneFromURL(instance.GetZone())
			instances = append(instances, instanceInfo{
				name: instance.GetName(),
				zone: zone,
			})
			log.Printf("  Found VM Instance: %s (zone: %s, status: %s)",
				instance.GetName(), zone, instance.GetStatus())
		}
	}

	if len(instances) == 0 {
		log.Printf("[%s] No VM instances found", projectID)
		return nil
	}

	if dryRun {
		log.Printf("[%s] Would delete %d VM instances", projectID, len(instances))
		return nil
	}

	// Delete all instances in parallel
	log.Printf("[%s] Deleting %d VM instances in parallel...", projectID, len(instances))
	var wg sync.WaitGroup
	for _, inst := range instances {
		wg.Add(1)
		go func(name, zone string) {
			defer wg.Done()
			deleteReq := &computepb.DeleteInstanceRequest{
				Project:  projectID,
				Zone:     zone,
				Instance: name,
			}
			op, err := client.Delete(ctx, deleteReq)
			if err != nil {
				log.Printf("  ERROR deleting instance %s: %v", name, err)
				return
			}
			if err := op.Wait(ctx); err != nil {
				log.Printf("  ERROR waiting for deletion of %s: %v", name, err)
			} else {
				log.Printf("  ✓ Deleted VM instance: %s", name)
			}
		}(inst.name, inst.zone)
	}
	wg.Wait()
	log.Printf("[%s] All VM deletions complete", projectID)

	return nil
}

func deleteDisks(ctx context.Context, projectID string) error {
	log.Printf("[%s] Checking disks...", projectID)

	client, err := compute.NewDisksRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create disks client: %w", err)
	}
	defer client.Close()

	// Use aggregated list to get all disks across all zones
	req := &computepb.AggregatedListDisksRequest{
		Project: projectID,
	}

	diskCount := 0
	it := client.AggregatedList(ctx, req)
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("error listing disks: %w", err)
		}

		for _, disk := range pair.Value.Disks {
			// Extract zone from the zone URL
			zone := extractZoneFromURL(disk.GetZone())

			// Skip disks that are attached to instances
			if len(disk.GetUsers()) > 0 {
				log.Printf("  Skipping disk %s (attached to instances)", disk.GetName())
				continue
			}

			diskCount++
			log.Printf("  Found Disk: %s (zone: %s, size: %d GB)",
				disk.GetName(), zone, disk.GetSizeGb())

			if !dryRun {
				deleteReq := &computepb.DeleteDiskRequest{
					Project: projectID,
					Zone:    zone,
					Disk:    disk.GetName(),
				}
				op, err := client.Delete(ctx, deleteReq)
				if err != nil {
					log.Printf("  ERROR deleting disk %s: %v", disk.GetName(), err)
					continue
				}
				if err := op.Wait(ctx); err != nil {
					log.Printf("  ERROR waiting for deletion of %s: %v", disk.GetName(), err)
				} else {
					log.Printf("  ✓ Deleted disk: %s", disk.GetName())
				}
			}
		}
	}

	if diskCount == 0 {
		log.Printf("[%s] No unattached disks found", projectID)
	} else if dryRun {
		log.Printf("[%s] Would delete %d disks", projectID, diskCount)
	}

	return nil
}

func releaseStaticIPs(ctx context.Context, projectID string) error {
	log.Printf("[%s] Checking static IP addresses...", projectID)

	client, err := compute.NewAddressesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create addresses client: %w", err)
	}
	defer client.Close()

	// Use aggregated list to get all addresses across all regions
	req := &computepb.AggregatedListAddressesRequest{
		Project: projectID,
	}

	ipCount := 0
	it := client.AggregatedList(ctx, req)
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("error listing addresses: %w", err)
		}

		for _, address := range pair.Value.Addresses {
			// Extract region from the region URL
			region := extractRegionFromURL(address.GetRegion())

			// Skip addresses that are in use
			if address.GetStatus() == "IN_USE" {
				log.Printf("  Skipping address %s (in use)", address.GetName())
				continue
			}

			ipCount++
			log.Printf("  Found Static IP: %s (region: %s, address: %s, status: %s)",
				address.GetName(), region, address.GetAddress(), address.GetStatus())

			if !dryRun {
				deleteReq := &computepb.DeleteAddressRequest{
					Project: projectID,
					Region:  region,
					Address: address.GetName(),
				}
				op, err := client.Delete(ctx, deleteReq)
				if err != nil {
					log.Printf("  ERROR releasing address %s: %v", address.GetName(), err)
					continue
				}
				if err := op.Wait(ctx); err != nil {
					log.Printf("  ERROR waiting for release of %s: %v", address.GetName(), err)
				} else {
					log.Printf("  ✓ Released static IP: %s", address.GetName())
				}
			}
		}
	}

	// Also check global addresses
	globalReq := &computepb.ListGlobalAddressesRequest{
		Project: projectID,
	}

	globalClient, err := compute.NewGlobalAddressesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create global addresses client: %w", err)
	}
	defer globalClient.Close()

	globalIt := globalClient.List(ctx, globalReq)
	for {
		globalAddr, err := globalIt.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("Error listing global addresses: %v", err)
			break
		}

		if globalAddr.GetStatus() == "IN_USE" {
			log.Printf("  Skipping global address %s (in use)", globalAddr.GetName())
			continue
		}

		ipCount++
		log.Printf("  Found Global Static IP: %s (address: %s, status: %s)",
			globalAddr.GetName(), globalAddr.GetAddress(), globalAddr.GetStatus())

		if !dryRun {
			deleteReq := &computepb.DeleteGlobalAddressRequest{
				Project: projectID,
				Address: globalAddr.GetName(),
			}
			op, err := globalClient.Delete(ctx, deleteReq)
			if err != nil {
				log.Printf("  ERROR releasing global address %s: %v", globalAddr.GetName(), err)
				continue
			}
			if err := op.Wait(ctx); err != nil {
				log.Printf("  ERROR waiting for release of %s: %v", globalAddr.GetName(), err)
			} else {
				log.Printf("  ✓ Released global static IP: %s", globalAddr.GetName())
			}
		}
	}

	if ipCount == 0 {
		log.Printf("[%s] No unused static IPs found", projectID)
	} else if dryRun {
		log.Printf("[%s] Would release %d static IPs", projectID, ipCount)
	}

	return nil
}

func deleteBuckets(ctx context.Context, projectID string) error {
	log.Printf("[%s] Checking storage buckets...", projectID)

	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create storage client: %w", err)
	}
	defer client.Close()

	it := client.Buckets(ctx, projectID)
	bucketCount := 0

	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("error listing buckets: %w", err)
		}

		bucketCount++
		log.Printf("  Found Bucket: %s (location: %s, storage class: %s)",
			attrs.Name, attrs.Location, attrs.StorageClass)

		if !dryRun {
			bucket := client.Bucket(attrs.Name)

			// Delete all objects in the bucket first
			log.Printf("  Deleting objects in bucket %s...", attrs.Name)
			objIt := bucket.Objects(ctx, nil)
			objCount := 0
			for {
				objAttrs, err := objIt.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					log.Printf("  ERROR listing objects in bucket %s: %v", attrs.Name, err)
					break
				}

				if err := bucket.Object(objAttrs.Name).Delete(ctx); err != nil {
					log.Printf("  ERROR deleting object %s: %v", objAttrs.Name, err)
				} else {
					objCount++
				}
			}

			if objCount > 0 {
				log.Printf("  Deleted %d objects from bucket %s", objCount, attrs.Name)
			}

			// Now delete the bucket
			if err := bucket.Delete(ctx); err != nil {
				log.Printf("  ERROR deleting bucket %s: %v", attrs.Name, err)
			} else {
				log.Printf("  ✓ Deleted bucket: %s", attrs.Name)
			}
		}
	}

	if bucketCount == 0 {
		log.Printf("[%s] No buckets found", projectID)
	} else if dryRun {
		log.Printf("[%s] Would delete %d buckets", projectID, bucketCount)
	}

	return nil
}

// Helper function to extract zone name from zone URL
// URL format: https://www.googleapis.com/compute/v1/projects/{project}/zones/{zone}
func extractZoneFromURL(zoneURL string) string {
	if zoneURL == "" {
		return ""
	}
	parts := strings.Split(zoneURL, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return zoneURL
}

// Helper function to extract region name from region URL
// URL format: https://www.googleapis.com/compute/v1/projects/{project}/regions/{region}
func extractRegionFromURL(regionURL string) string {
	if regionURL == "" {
		return ""
	}
	parts := strings.Split(regionURL, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return regionURL
}

func deleteServiceAccounts(ctx context.Context, projectID string) error {
	log.Printf("[%s] Checking service accounts...", projectID)

	iamService, err := iam.NewService(ctx)
	if err != nil {
		return fmt.Errorf("failed to create IAM service: %w", err)
	}

	// List all service accounts
	resp, err := iamService.Projects.ServiceAccounts.List("projects/" + projectID).Do()
	if err != nil {
		return fmt.Errorf("failed to list service accounts: %w", err)
	}

	// Filter service accounts that start with "vsa-sa-gcnv"
	var targetAccounts []*iam.ServiceAccount
	for _, sa := range resp.Accounts {
		// Extract the email local part (before @)
		emailParts := strings.Split(sa.Email, "@")
		if len(emailParts) > 0 && strings.HasPrefix(emailParts[0], "vsa-sa-gcnv") {
			targetAccounts = append(targetAccounts, sa)
			log.Printf("  Found Service Account: %s (%s)", sa.Email, sa.DisplayName)
		}
	}

	if len(targetAccounts) == 0 {
		log.Printf("[%s] No service accounts found with prefix 'vsa-sa-gcnv'", projectID)
		return nil
	}

	if dryRun {
		log.Printf("[%s] Would delete %d service accounts", projectID, len(targetAccounts))
		return nil
	}

	// Delete service accounts in parallel
	log.Printf("[%s] Deleting %d service accounts in parallel...", projectID, len(targetAccounts))
	var wg sync.WaitGroup
	for _, sa := range targetAccounts {
		wg.Add(1)
		go func(account *iam.ServiceAccount) {
			defer wg.Done()
			_, err := iamService.Projects.ServiceAccounts.Delete(account.Name).Do()
			if err != nil {
				log.Printf("  ERROR deleting service account %s: %v", account.Email, err)
			} else {
				log.Printf("  ✓ Deleted service account: %s", account.Email)
			}
		}(sa)
	}
	wg.Wait()
	log.Printf("[%s] All service account deletions complete", projectID)

	return nil
}
