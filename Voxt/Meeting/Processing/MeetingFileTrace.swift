import Darwin
import Foundation

/// Temporary file-task instrumentation. Remove these call sites independently of
/// processing logic. Debug defaults on; VOXT_FILE_TASK_TRACE=0 disables it and
/// VOXT_FILE_TASK_TRACE=1 enables it in Release. Uses existing bounded log storage.
nonisolated enum MeetingFileTrace {
    @TaskLocal static var taskID: UUID?

    static let isEnabled: Bool = {
        #if DEBUG
        let debugBuild = true
        #else
        let debugBuild = false
        #endif
        return resolvedEnabled(
            environmentValue: ProcessInfo.processInfo.environment["VOXT_FILE_TASK_TRACE"],
            debugBuild: debugBuild
        )
    }()

    static func resolvedEnabled(environmentValue: String?, debugBuild: Bool) -> Bool {
        switch environmentValue?.lowercased() {
        case "0", "false", "off": return false
        case "1", "true", "on": return true
        default: return debugBuild
        }
    }

    static func event(
        _ name: String,
        taskID explicitID: UUID? = nil,
        _ details: @autoclosure () -> String = ""
    ) {
        guard isEnabled, let id = explicitID ?? taskID else { return }
        VoxtLog.meeting("[FileTaskTrace] taskID=\(id), event=\(name), \(details()), \(resourceSnapshot())")
    }

    /// RSS is a process-wide instantaneous sample, not a task peak or GPU memory.
    private static func resourceSnapshot() -> String {
        var info = mach_task_basic_info()
        var count = mach_msg_type_number_t(MemoryLayout<mach_task_basic_info>.size / MemoryLayout<natural_t>.size)
        let result = withUnsafeMutablePointer(to: &info) { pointer in
            pointer.withMemoryRebound(to: integer_t.self, capacity: Int(count)) {
                task_info(mach_task_self_, task_flavor_t(MACH_TASK_BASIC_INFO), $0, &count)
            }
        }
        let rss = result == KERN_SUCCESS ? String(info.resident_size) : "unavailable"
        var vm = task_vm_info_data_t()
        var vmCount = mach_msg_type_number_t(MemoryLayout<task_vm_info_data_t>.size / MemoryLayout<natural_t>.size)
        let vmResult = withUnsafeMutablePointer(to: &vm) { pointer in
            pointer.withMemoryRebound(to: integer_t.self, capacity: Int(vmCount)) {
                task_info(mach_task_self_, task_flavor_t(TASK_VM_INFO), $0, &vmCount)
            }
        }
        let footprint = vmResult == KERN_SUCCESS ? String(vm.phys_footprint) : "unavailable"
        let compressed = vmResult == KERN_SUCCESS ? String(vm.compressed) : "unavailable"
        return "processRSSBytes=\(rss), physicalFootprintBytes=\(footprint), compressedBytes=\(compressed), thermalState=\(ProcessInfo.processInfo.thermalState.rawValue)"
    }
}
