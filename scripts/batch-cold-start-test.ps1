# batch-cold-start-test.ps1
param(
    [Parameter(Mandatory=$true)]
    [string]$ResourceGroup,
    
    [Parameter(Mandatory=$true)]
    [string]$VMSSName,
    
    [string[]]$InstanceIds = @("0", "1", "2", "3"),
    [int]$Iterations = 5,
    [int]$DelayBetweenTests = 30,
    [switch]$ParallelTests
)

$allResults = @()
$csvFile = "vmss-cold-start-results-$(Get-Date -Format 'yyyyMMdd-HHmmss').csv"

Write-Host "=== VMSS Cold Start Batch Test ===" -ForegroundColor Green
Write-Host "VMSS: $VMSSName" -ForegroundColor Yellow
Write-Host "Resource Group: $ResourceGroup" -ForegroundColor Yellow
Write-Host "Instances: $($InstanceIds -join ', ')" -ForegroundColor Yellow
Write-Host "Iterations: $Iterations" -ForegroundColor Yellow
Write-Host "Results will be saved to: $csvFile" -ForegroundColor Yellow
Write-Host ""

for ($iteration = 1; $iteration -le $Iterations; $iteration++) {
    Write-Host "=== Iteration $iteration of $Iterations ===" -ForegroundColor Cyan
    
    if ($ParallelTests) {
        # Run tests in parallel for all instances
        $jobs = @()
        foreach ($instanceId in $InstanceIds) {
            $scriptBlock = {
                param($ResourceGroup, $VMSSName, $InstanceId)
                
                # Build parameter hashtable
                $params = @{
                    ResourceGroup = $ResourceGroup
                    VMSSName = $VMSSName
                    InstanceId = $InstanceId
                }
                
                # Call the script with proper parameters
                & ".\measure-vmss-cold-start.ps1" @params
            }
            
            $job = Start-Job -ScriptBlock $scriptBlock -ArgumentList $ResourceGroup, $VMSSName, $instanceId
            $jobs += $job
        }
        
        # Wait for all jobs to complete
        $results = $jobs | ForEach-Object { 
            $result = Receive-Job $_ -Wait
            Remove-Job $_
            return $result
        }
    } else {
        # Run tests sequentially
        $results = @()
        foreach ($instanceId in $InstanceIds) {
            Write-Host "Testing instance $instanceId..." -ForegroundColor White
            
            # Build parameter hashtable to avoid parameter conflicts
            $params = @{
                ResourceGroup = $ResourceGroup
                VMSSName = $VMSSName
                InstanceId = $instanceId
            }
            
            # Call the script with splatted parameters
            $result = & ".\measure-vmss-cold-start.ps1" @params
            $results += $result
            
            if ($instanceId -ne $InstanceIds[-1] -and $DelayBetweenTests -gt 0) {
                Write-Host "Waiting $DelayBetweenTests seconds before next test..." -ForegroundColor Gray
                Start-Sleep -Seconds $DelayBetweenTests
            }
        }
    }
    
    # Process results
    foreach ($result in $results) {
        if ($result) {
            $resultObj = [PSCustomObject]@{
                Iteration = $iteration
                InstanceId = $result.InstanceId
                Success = $result.Success
                TotalTimeSeconds = if ($result.TotalTime) { [math]::Round($result.TotalTime, 2) } else { 0 }
                ApiTimeSeconds = if ($result.ApiTime) { [math]::Round($result.ApiTime, 2) } else { 0 }
                BootTimeSeconds = if ($result.BootTime) { [math]::Round($result.BootTime, 2) } else { 0 }
                FinalState = if ($result.FinalState) { $result.FinalState } else { "PowerState/running" }
                Timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
            }
            
            $allResults += $resultObj
            
            # Display result
            if ($result.Success) {
                Write-Host "✓ Instance $($result.InstanceId): $([math]::Round($result.TotalTime, 2))s total ($([math]::Round($result.BootTime, 2))s boot)" -ForegroundColor Green
            } else {
                Write-Host "✗ Instance $($result.InstanceId): FAILED ($($result.FinalState))" -ForegroundColor Red
            }
        }
    }
    
    Write-Host ""
}

# Save results to CSV
$allResults | Export-Csv -Path $csvFile -NoTypeInformation -Encoding UTF8
Write-Host "Results saved to: $csvFile" -ForegroundColor Green

# Display summary statistics
$successfulTests = $allResults | Where-Object { $_.Success -eq $true }
if ($successfulTests.Count -gt 0) {
    Write-Host "=== SUMMARY STATISTICS ===" -ForegroundColor Green
    Write-Host "Successful tests: $($successfulTests.Count) / $($allResults.Count)" -ForegroundColor Yellow
    Write-Host "Success rate: $([math]::Round(($successfulTests.Count / $allResults.Count) * 100, 1))%" -ForegroundColor Yellow
    Write-Host ""
    
    Write-Host "Total Start Times:" -ForegroundColor White
    Write-Host "  Average: $([math]::Round(($successfulTests.TotalTimeSeconds | Measure-Object -Average).Average, 2)) seconds" -ForegroundColor White
    Write-Host "  Minimum: $([math]::Round(($successfulTests.TotalTimeSeconds | Measure-Object -Minimum).Minimum, 2)) seconds" -ForegroundColor White
    Write-Host "  Maximum: $([math]::Round(($successfulTests.TotalTimeSeconds | Measure-Object -Maximum).Maximum, 2)) seconds" -ForegroundColor White
    
    Write-Host "Boot Times (excluding API calls):" -ForegroundColor White
    Write-Host "  Average: $([math]::Round(($successfulTests.BootTimeSeconds | Measure-Object -Average).Average, 2)) seconds" -ForegroundColor White
    Write-Host "  Minimum: $([math]::Round(($successfulTests.BootTimeSeconds | Measure-Object -Minimum).Minimum, 2)) seconds" -ForegroundColor White
    Write-Host "  Maximum: $([math]::Round(($successfulTests.BootTimeSeconds | Measure-Object -Maximum).Maximum, 2)) seconds" -ForegroundColor White
}

# Group by instance for per-instance stats
$groupedResults = $successfulTests | Group-Object -Property InstanceId
foreach ($group in $groupedResults) {
    Write-Host "Instance $($group.Name) average: $([math]::Round(($group.Group.TotalTimeSeconds | Measure-Object -Average).Average, 2)) seconds" -ForegroundColor Cyan
}