function make_initial_temperature(nx::Int, ny::Int)
    u = zeros(Float64, nx, ny)
    for j in 1:ny
        for i in 1:nx
            dx = i - nx ÷ 2
            dy = j - ny ÷ 2
            r2 = dx * dx + dy * dy
            if r2 < 900
                u[i, j] = 100.0
            elseif r2 < 3600
                u[i, j] = 40.0
            end
        end
    end
    u
end

function diffuse_slow(u0::Matrix{Float64}, alpha::Float64, steps::Int)
    u = copy(u0)
    for _ in 1:steps
        next = copy(u)
        next[2:end-1, 2:end-1] =
            u[2:end-1, 2:end-1] .+
            alpha .* (
                u[1:end-2, 2:end-1] .+
                u[3:end, 2:end-1] .+
                u[2:end-1, 1:end-2] .+
                u[2:end-1, 3:end] .-
                4.0 .* u[2:end-1, 2:end-1]
            )
        u = next
    end
    u
end

u0 = make_initial_temperature(500, 500)
@time u = diffuse_slow(u0, 0.12, 260)
println("center temperature = ", u[250, 250], ", corner temperature = ", u[1, 1])
