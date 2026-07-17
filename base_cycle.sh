
# REBUILD: Re-create any [MISSING] links using the registry data
li_rebuild() {
    [ ! -s "$LI_REGISTRY" ] && { echo "Registry empty. Nothing to rebuild."; return 1; }
    
    local count=0
    local fail=0

    echo ">>> Scanning registry to rebuild missing links..."
    echo "------------------------------------------------"

    while IFS='|' read -r dest src type cat || [ -n "$dest" ]; do
        # Check if the link file itself is missing
        if [ ! -L "$dest" ]; then
            # Check if the source exists before trying to link
            if [ -e "$src" ]; then
                # Ensure the parent directory of the link exists (important for new PCs)
                mkdir -p "$(dirname "$dest")"
                
                ln -s "$src" "$dest"
                echo -e "\033[1;32m[RESTORED]\033[0m $(basename "$dest") in $cat"
                ((count++))
            else
                echo -e "\033[1;31m[FAILED]\033[0m Source not found: $src"
                ((fail++))
            fi
        fi
    done < "$LI_REGISTRY"

    echo "------------------------------------------------"
    echo "REBUILD COMPLETE:"
    echo " - Links Restored : $count"
    echo " - Sources Missing: $fail (Check if your external drive is mounted)"
}

_li_cycle_helper() {
    local suffix="$1"
    local link_name="$2"
    
    # 1. Find all available cycle folders
    local folders=($(ls -d cycle_*_${suffix} 2>/dev/null))
    local options=()
    for f in "${folders[@]}"; do
        local name="${f#cycle_}"
        options+=("${name%_${suffix}}")
    done

    if [ ${#options[@]} -eq 0 ]; then
        echo "❌ No 'cycle_*_${suffix}' folders found in this directory."
        return 1
    fi

    # 2. Identify what link_name is currently pointing to
    if [ -L "$link_name" ]; then
        local current_target=$(readlink "$link_name")
        local current_name="${current_target#cycle_}"
        current_name="${current_name%_${suffix}}"
        echo "📍 Currently active: $current_name"
    elif [ -d "$link_name" ]; then
        echo "⚠️  Warning: '$link_name' is a real directory, not a symlink."
        echo "To use this script, move it to 'cycle_something_${suffix}' first."
        return 1
    else
        echo "⚪ No active '$link_name' link found."
    fi

    # 3. Selection Menu
    echo "Select a folder to link to '$link_name':"
    select selected_name in "${options[@]}"; do
        if [ -n "$selected_name" ]; then
            local target="cycle_${selected_name}_${suffix}"
            ln -sfn "$target" "$link_name"
            echo "✅ '$link_name' is now pointing to: $selected_name"
            break
        else
            echo "🚫 Invalid selection."
        fi
    done
}

li_cycle_data() {
    _li_cycle_helper "data" "data"
}

li_cycle_storage() {
    _li_cycle_helper "globalStorage" "globalStorage"
}