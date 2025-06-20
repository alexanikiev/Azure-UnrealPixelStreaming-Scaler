# measure-vmss-cold-start.ps1
param(
    [Parameter(Mandatory=$true)]
    [string]$ResourceGroup,
    
    [Parameter(Mandatory=$true)]
    [string]$VMSSName,
    
    [Parameter(Mandatory=$true)]
    [string]$InstanceId,
    
    [int]$TimeoutMinutes = 15,
    [int]$PollIntervalSeconds = 5
)

# Function to get VM power state
function Get-VMPowerState {
    param($ResourceGroup, $VMSSName, $InstanceId)
    
    $result = az vmss get-instance-view --resource-group $ResourceGroup --name $VMSSName --instance-id $InstanceId --query "statuses[?starts_with(code, 'PowerState/')].code" -o tsv 2>$null
    if ($LASTEXITCODE -ne 0) {
        return $null
    }
    return $result.Trim()
}

# Function to log with timestamp
function Write-TimestampLog {
    param($Message)
    
    $timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss.fff"
    $logMessage = "[$timestamp] $Message"
    Write-Host $logMessage
    
    # Also write to log file
    $logMessage | Out-File -FilePath "vmss-timing-log.txt" -Append -Encoding UTF8
}

# Function to ensure VM is deallocated
function Ensure-VMDeallocated {
    param($ResourceGroup, $VMSSName, $InstanceId)
    
    $currentState = Get-VMPowerState -ResourceGroup $ResourceGroup -VMSSName $VMSSName -InstanceId $InstanceId
    
    if ($currentState -eq "PowerState/deallocated") {
        Write-TimestampLog "VM $InstanceId is already deallocated"
        return $true
    }
    
    Write-TimestampLog "Deallocating VM $InstanceId (current state: $currentState)"
    
    $result = az vmss deallocate --resource-group $ResourceGroup --name $VMSSName --instance-ids $InstanceId 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-TimestampLog "ERROR: Failed to deallocate VM: $result"
        return $false
    }
    
    # Wait for deallocation
    $timeout = (Get-Date).AddMinutes(10)
    do {
        Start-Sleep -Seconds 3
        $state = Get-VMPowerState -ResourceGroup $ResourceGroup -VMSSName $VMSSName -InstanceId $InstanceId
        Write-TimestampLog "Waiting for deallocation... Current state: $state"
    } while ($state -ne "PowerState/deallocated" -and (Get-Date) -lt $timeout)
    
    if ($state -eq "PowerState/deallocated") {
        Write-TimestampLog "VM $InstanceId successfully deallocated"
        return $true
    } else {
        Write-TimestampLog "ERROR: VM failed to deallocate within timeout"
        return $false
    }
}

# Main execution
Write-TimestampLog "=== Starting Cold Start Measurement ==="
Write-TimestampLog "VMSS: $VMSSName, Instance: $InstanceId, Resource Group: $ResourceGroup"

# Step 1: Ensure VM is deallocated
if (-not (Ensure-VMDeallocated -ResourceGroup $ResourceGroup -VMSSName $VMSSName -InstanceId $InstanceId)) {
    Write-TimestampLog "ERROR: Failed to prepare VM for cold start test"
    exit 1
}

# Step 2: Wait a moment to ensure complete deallocation
Write-TimestampLog "Waiting 10 seconds to ensure complete deallocation..."
Start-Sleep -Seconds 10

# Step 3: Start timing and begin VM start
Write-TimestampLog "Starting cold start measurement..."
$startTime = Get-Date

# Start the VM
Write-TimestampLog "Issuing start command..."
$startResult = az vmss start --resource-group $ResourceGroup --name $VMSSName --instance-ids $InstanceId 2>&1

if ($LASTEXITCODE -ne 0) {
    Write-TimestampLog "ERROR: Failed to start VM: $startResult"
    exit 1
}

$apiCallTime = Get-Date
$apiDuration = ($apiCallTime - $startTime).TotalSeconds
Write-TimestampLog "Azure API call completed in $([math]::Round($apiDuration, 2)) seconds"

# Step 4: Poll for running state
$timeout = $startTime.AddMinutes($TimeoutMinutes)
Write-TimestampLog "Polling for VM running state (timeout: $TimeoutMinutes minutes, interval: $PollIntervalSeconds seconds)..."

$lastState = ""
do {
    Start-Sleep -Seconds $PollIntervalSeconds
    $currentState = Get-VMPowerState -ResourceGroup $ResourceGroup -VMSSName $VMSSName -InstanceId $InstanceId
    $elapsed = ((Get-Date) - $startTime).TotalSeconds
    
    if ($currentState -ne $lastState) {
        Write-TimestampLog "State change: $lastState -> $currentState (elapsed: $([math]::Round($elapsed, 2))s)"
        $lastState = $currentState
    } else {
        Write-TimestampLog "Current state: $currentState (elapsed: $([math]::Round($elapsed, 2))s)"
    }
    
    if ($currentState -eq "PowerState/running") {
        $totalTime = ((Get-Date) - $startTime).TotalSeconds
        $bootTime = $totalTime - $apiDuration
        
        Write-TimestampLog "=== SUCCESS: VM is now running! ==="
        Write-TimestampLog "Total cold start time: $([math]::Round($totalTime, 2)) seconds"
        Write-TimestampLog "API call time: $([math]::Round($apiDuration, 2)) seconds"
        Write-TimestampLog "Boot time: $([math]::Round($bootTime, 2)) seconds"
        
        # Return structured data for batch processing
        return @{
            Success = $true
            TotalTime = $totalTime
            ApiTime = $apiDuration
            BootTime = $bootTime
            InstanceId = $InstanceId
        }
    }
    
} while ((Get-Date) -lt $timeout)

# Timeout reached
$totalTime = ((Get-Date) - $startTime).TotalSeconds
Write-TimestampLog "=== TIMEOUT: VM did not start within $TimeoutMinutes minutes ==="
Write-TimestampLog "Final state: $currentState"
Write-TimestampLog "Total elapsed time: $([math]::Round($totalTime, 2)) seconds"

return @{
    Success = $false
    TotalTime = $totalTime
    ApiTime = $apiDuration
    BootTime = 0
    InstanceId = $InstanceId
    FinalState = $currentState
}