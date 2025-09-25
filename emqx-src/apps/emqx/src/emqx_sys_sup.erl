%%--------------------------------------------------------------------
%% Copyright (c) 2018-2025 EMQ Technologies Co., Ltd. All Rights Reserved.
%%--------------------------------------------------------------------

-module(emqx_sys_sup).

-behaviour(supervisor).

-export([start_link/0]).
-export([init/1]).

start_link() ->
    supervisor:start_link({local, ?MODULE}, ?MODULE, []).

init([]) ->
    OsMon =
        case emqx_os_mon:is_os_check_supported() of
            true -> [child_spec(emqx_os_mon), child_spec(emqx_cpu_sup_worker)];
            false -> []
        end,
    LoadAlarmHandler = #{
        id => load_alarm_handler,
        start => {emqx_alarm_handler, load, []},
        restart => transient,
        shutdown => 1_000,
        type => worker
    },
    Children =
        [
            child_spec(emqx_alarm),
            LoadAlarmHandler,
            child_spec(emqx_sys),
            child_spec(emqx_sys_mon),
            child_spec(emqx_vm_mon),
            child_spec(emqx_broker_mon)
        ] ++ OsMon ++ [child_spec(emqx_corrupt_namespace_config_checker)],
    {ok, {{one_for_one, 10, 100}, Children}}.

%%--------------------------------------------------------------------
%% Internal functions
%%--------------------------------------------------------------------

child_spec(Mod) ->
    child_spec(Mod, []).

child_spec(Mod, Args) ->
    #{
        id => Mod,
        start => {Mod, start_link, Args},
        restart => permanent,
        shutdown => 5000,
        type => worker,
        modules => [Mod]
    }.
