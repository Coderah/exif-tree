#!/bin/bash

# Function to display usage
usage() {
    echo "Usage: $0 <target_directory> [--dry-run]"
    echo "  <target_directory>: The directory to search for JPG images and where categorized images will be placed."
    echo "  --dry-run: (Optional) If specified, the script will only show what it would do without actually moving files."
    exit 1
}

# Parse command-line arguments
target_dir=""
dry_run=false

if [ "$#" -lt 1 ]; then
    usage
fi

target_dir="$1"

if [ "$#" -ge 2 ] && [ "$2" == "--dry-run" ]; then
    dry_run=true
elif [ "$#" -ge 2 ]; then
    usage
fi

# Check if target directory exists
if [ ! -d "$target_dir" ]; then
    echo "Error: Target directory '$target_dir' not found."
    exit 1
fi

# Define the "Uncategorized" directory within the target directory
uncategorized_dir="$target_dir/Uncategorized"
if [ ! -d "$uncategorized_dir" ]; then
    echo "Creating Uncategorized directory: $uncategorized_dir"
    if ! $dry_run; then
        mkdir -p "$uncategorized_dir"
    fi
fi

echo "--- Starting Image Categorization ---"
echo "Target Directory: $target_dir"
if $dry_run; then
    echo "DRY RUN MODE: No files will be moved."
else
    echo "Actual run: Files will be moved."
fi
echo "-------------------------------------"

# Find all JPGs and process them
# We use -depth to process files before directories, important if moving files within the same tree
# Also, exclude the Uncategorized directory to prevent infinite loops or issues
find "$target_dir" -depth -type f -iname "*.jpg" ! -path "$uncategorized_dir/*" -print0 | while IFS= read -r -d $'\0' jpg_file; do
    echo "Processing: $jpg_file"

    # Get Hierarchical Subject using exiftool
    hierarchical_subject=$(exiftool -s3 -HierarchicalSubject "$jpg_file" 2>/dev/null)

    if [[ -n "$hierarchical_subject" ]]; then
        best_subject=""
        max_depth=-1

        while IFS= read -r line; do
            current_depth=$(echo "$line" | grep -o '|' | wc -l)
            if (( current_depth > max_depth )); then
                max_depth=$current_depth
                best_subject="$line"
            fi
        done <<< "$hierarchical_subject"

        if [[ -n "$best_subject" ]]; then
            # Replace '|' with '/' to create directory structure
            # Sanitize the subject to be safe for directory names
            sanitized_subject=$(echo "$best_subject" | sed 's/|/\//g' | sed 's/[^a-zA-Z0-9_\-\/ ]/_/g') # Basic sanitization

            destination_dir="$target_dir/$sanitized_subject"
            echo "  Found Hierarchical Subject (most depth): '$best_subject'"
            echo "  Destination Directory: '$destination_dir'"

            if ! $dry_run; then
                mkdir -p "$destination_dir"
                mv "$jpg_file" "$destination_dir/"
                echo "  Moved '$jpg_file' to '$destination_dir/'"
            else
                echo "  (Dry Run) Would create directory '$destination_dir' and move '$jpg_file' to it."
            fi
        else
            echo "  No suitable Hierarchical Subject found with depth. Moving to Uncategorized."
            if ! $dry_run; then
                mv "$jpg_file" "$uncategorized_dir/"
                echo "  Moved '$jpg_file' to '$uncategorized_dir/'"
            else
                echo "  (Dry Run) Would move '$jpg_file' to '$uncategorized_dir/'"
            fi
        fi
    else
        echo "  No Hierarchical Subject found. Moving to Uncategorized."
        if ! $dry_run; then
            mv "$jpg_file" "$uncategorized_dir/"
            echo "  Moved '$jpg_file' to '$uncategorized_dir/'"
        else
            echo "  (Dry Run) Would move '$jpg_file' to '$uncategorized_dir/'"
        fi
    fi
    echo "-------------------------------------"
done

echo "--- Categorization Complete ---"