using Base64
using InteractiveUtils
using Test

const PREFIX_REQ = "SYSLABMCP-REQ"
const PREFIX_RES = "SYSLABMCP-RES"

function b64(s::AbstractString)
    return base64encode(Vector{UInt8}(codeunits(s)))
end

function unb64(s::AbstractString)
    isempty(s) && return ""
    return String(base64decode(s))
end

function response_line(id::AbstractString, stdout::AbstractString, stderr::AbstractString, result::AbstractString, errtype::AbstractString, errmsg::AbstractString, stack::AbstractString)
    return join((
        PREFIX_RES,
        id,
        isempty(errmsg) ? "true" : "false",
        b64(stdout),
        b64(stderr),
        b64(result),
        b64(errtype),
        b64(errmsg),
        b64(stack),
    ), '\t')
end

function parse_request(line::String)
    parts = split(chomp(line), '\t'; keepempty=true)
    if length(parts) != 5 || parts[1] != PREFIX_REQ
        return nothing
    end
    return (
        id = parts[2],
        method = parts[3],
        cwd = unb64(parts[4]),
        payload = unb64(parts[5]),
    )
end

function check_source(source::String)
    Meta.parse("begin\n" * source * "\nend")
    return "Syntax check passed."
end

function detect_environment_text()
    lines = String[]
    push!(lines, "session_pid: $(getpid())")
    push!(lines, "julia_version: $(VERSION)")
    push!(lines, "threads: $(Threads.nthreads())")
    push!(lines, "pwd: $(pwd())")
    push!(lines, "bindir: $(Sys.BINDIR)")
    push!(lines, "depot_path: $(join(DEPOT_PATH, ';'))")
    push!(lines, "python: $(get(ENV, "PYTHON", ""))")
    push!(lines, "ty_conda3: $(get(ENV, "TY_CONDA3", ""))")
    push!(lines, "julia_depot_path: $(get(ENV, "JULIA_DEPOT_PATH", ""))")
    return join(lines, "\n")
end

function execute_request(method::AbstractString, payload::AbstractString)
    if method == "health"
        return "ok"
    elseif method == "detect_environment"
        return detect_environment_text()
    elseif method == "check_code"
        source = read(payload, String)
        return check_source(source)
    elseif method == "evaluate"
        expr = Meta.parse("begin\n" * payload * "\nend")
        value = Core.eval(Main, expr)
        return repr(value)
    elseif method == "run_file"
        value = Base.include(Main, payload)
        return repr(value)
    elseif method == "run_test_file"
        value = Base.include(Main, payload)
        return "Test file executed. Final value: " * repr(value)
    else
        error("Unknown bridge method: " * method)
    end
end

function main()
    while !eof(stdin)
        raw = readline(stdin)
        req = parse_request(raw)
        req === nothing && continue

        if !isempty(req.cwd)
            cd(req.cwd)
        end

        stdout_io = IOBuffer()
        stderr_io = IOBuffer()
        result_text = ""
        errtype = ""
        errmsg = ""
        stack = ""

        try
            stdout_path, stdout_file = mktemp()
            stderr_path, stderr_file = mktemp()
            close(stdout_file)
            close(stderr_file)

            open(stdout_path, "w") do outio
                open(stderr_path, "w") do errio
                    redirect_stdout(() -> begin
                        redirect_stderr(() -> begin
                            result_text = execute_request(req.method, req.payload)
                        end, errio)
                    end, outio)
                end
            end

            write(stdout_io, read(stdout_path, String))
            write(stderr_io, read(stderr_path, String))
            rm(stdout_path; force=true)
            rm(stderr_path; force=true)
        catch err
            errtype = string(typeof(err))
            errmsg = sprint(showerror, err)
            stack = sprint(io -> Base.showerror(io, err, catch_backtrace()))
        end

        println(stdout, response_line(
            req.id,
            String(take!(stdout_io)),
            String(take!(stderr_io)),
            result_text,
            errtype,
            errmsg,
            stack,
        ))
        flush(stdout)
    end
end

main()
