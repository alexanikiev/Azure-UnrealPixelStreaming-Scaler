$flag = "C:\Windows\Temp\FirstBootComplete.flag"

if (-not (Test-Path $flag)) {
    # Check VM's previous state using Azure Instance Metadata Service
    $metadata = Invoke-RestMethod -Headers @{"Metadata"="true"} -Method GET -Uri "http://169.254.169.254/metadata/instance?api-version=2021-02-01"
    $vmState = $metadata.compute.vmState

    # Set environment variable regardless of state
    [Environment]::SetEnvironmentVariable("MATCHMAKER_SERVER", "0.0.0.0:0", "Machine") # your IP and port here

    # Mark first boot complete
    New-Item -Path $flag -ItemType File -Force

    # Only schedule shutdown if VM wasn't previously deallocated
    if ($vmState -ne "Deallocated") {
        # Create shutdown task (runs 2 mins from now)
        $startTime = (Get-Date).AddMinutes(2).ToString("HH:mm")
        schtasks /Create /TN "ShutdownAfterFirstBoot" /TR "powershell -Command Stop-Computer -Force" /SC ONCE /ST $startTime /RL HIGHEST /RU SYSTEM
    }
}
